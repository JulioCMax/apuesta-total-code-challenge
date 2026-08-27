package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// ErrUserAlreadyExists is returned by PutUserIfAbsent when a profile item
// already exists at the same partition key: a re-run seed must never
// clobber a played-with balance (design.md's seeding contract).
var ErrUserAlreadyExists = errors.New("dynamo: user already exists")

// entityTypeProfile is the entityType attribute value stamped on every
// user profile item.
const entityTypeProfile = "Profile"

// UserRepository implements application/auth.UserRepository against the
// single-table design's user profile item (PK=USER#<id>, SK=PROFILE) and
// its EmailIndex GSI.
type UserRepository struct {
	client *dynamodb.Client
	table  string
}

// NewUserRepository builds a UserRepository backed by client, targeting
// table.
func NewUserRepository(client *dynamodb.Client, table string) *UserRepository {
	return &UserRepository{client: client, table: table}
}

// PutUserIfAbsent inserts u's profile item unless one already exists at
// the same partition key. cmd/seed (Phase 13) uses this to seed demo users
// idempotently on every boot; SEED_RESET is handled by that caller, not
// here.
func (r *UserRepository) PutUserIfAbsent(ctx context.Context, u account.User) error {
	_, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.table),
		Item:                userItemAttrs(u),
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err != nil {
		var condFailed *types.ConditionalCheckFailedException
		if errors.As(err, &condFailed) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("dynamo: put user: %w", err)
	}
	return nil
}

// PutUser inserts or overwrites u's profile item unconditionally. cmd/seed
// (Phase 13) uses this only when SEED_RESET=true — the normal seeding path
// is PutUserIfAbsent, which never clobbers a played-with balance.
func (r *UserRepository) PutUser(ctx context.Context, u account.User) error {
	if _, err := r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(r.table),
		Item:      userItemAttrs(u),
	}); err != nil {
		return fmt.Errorf("dynamo: put user (unconditional): %w", err)
	}
	return nil
}

// SeedUser writes a demo profile identified by its EMAIL rather than by
// the id the caller happens to have generated, and is the only entry point
// cmd/seed should use.
//
// The distinction is not academic. A demo account's identity, for seeding
// purposes, is its email: that is what the seed list is keyed by and what
// a re-run means to re-seed. Its ULID is an internal detail minted on the
// spot. Writing straight through PutUserIfAbsent with a fresh ULID makes
// the attribute_not_exists(PK) condition meaningless — the key never
// existed, so the guard always passes — and every boot adds another
// profile for the same email. Login resolves through the email index and
// then returns whichever duplicate the index yields, so balances appear to
// change at random.
//
// So the existing profile is looked up first:
//
//   - absent: insert, still conditionally, so two seeders racing at boot
//     cannot both create it.
//   - present and reset is false: report ErrUserAlreadyExists and touch
//     nothing, which is what protects a played-with balance.
//   - present and reset is true: adopt the stored id and overwrite that
//     exact item, so SEED_RESET restores the balance instead of burying
//     the spent profile under a new one.
func (r *UserRepository) SeedUser(ctx context.Context, u account.User, reset bool) error {
	existing, err := r.FindByEmail(ctx, u.Email)
	switch {
	case err == nil:
		if !reset {
			return ErrUserAlreadyExists
		}
		u.ID = existing.ID
		return r.PutUser(ctx, u)
	case errors.Is(err, account.ErrInvalidCredentials):
		// FindByEmail deliberately reports an unknown email with the same
		// error a wrong password produces, so a caller can never learn
		// whether an address exists. Here it simply means "not seeded yet".
		return r.PutUserIfAbsent(ctx, u)
	default:
		return err
	}
}

// FindByEmail queries the EmailIndex GSI. The GSI's projection is
// deliberately narrow — INCLUDE [userId, passwordHash, balance]
// (design.md) — because that is exactly what application/auth.Login
// needs (verify the password, issue a token carrying the id; the email
// itself is already known from the request). Currency and CreatedAt are
// therefore always zero-valued on the returned User: a caller needing the
// full profile must read it via Balance's PK/SK GetItem path instead. An
// unseeded email returns account.ErrInvalidCredentials — the same error a
// wrong password produces — so a caller can never learn whether an email
// exists (spec: auth-and-balance/Demo User Login Issuing JWT).
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (account.User, error) {
	out, err := r.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		IndexName:              aws.String("EmailIndex"),
		KeyConditionExpression: aws.String("GSI1PK = :gsi1pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":gsi1pk": &types.AttributeValueMemberS{Value: EmailGSI1PK(email)},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return account.User{}, fmt.Errorf("dynamo: find by email: %w", err)
	}
	if len(out.Items) == 0 {
		return account.User{}, account.ErrInvalidCredentials
	}

	item := out.Items[0]
	userID, ok := attrString(item, "userId")
	if !ok {
		return account.User{}, fmt.Errorf("dynamo: email index item missing userId")
	}
	passwordHash, _ := attrString(item, "passwordHash")
	balance, err := UnmarshalMoney(item["balance"])
	if err != nil {
		return account.User{}, fmt.Errorf("dynamo: email index item: %w", err)
	}

	return account.User{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Balance:      balance,
	}, nil
}

// Balance reads the current balance from the user's profile item.
//
// ConsistentRead is mandatory here, not an optimisation. DynamoDB's
// default eventually-consistent GetItem may serve a replica that has not
// yet applied the debit the placement transaction just committed, which
// would render a stale balanceAfter right after a successful placement —
// the single most confusing thing this API could report.
func (r *UserRepository) Balance(ctx context.Context, userID string) (money.Money, error) {
	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: UserPK(userID)},
			"SK": &types.AttributeValueMemberS{Value: ProfileSK()},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return money.Money{}, fmt.Errorf("dynamo: balance: %w", err)
	}
	if out.Item == nil {
		return money.Money{}, fmt.Errorf("dynamo: balance: user %q not found", userID)
	}
	balAV, ok := out.Item["balance"]
	if !ok {
		return money.Money{}, fmt.Errorf("dynamo: balance: user %q missing balance attribute", userID)
	}
	return UnmarshalMoney(balAV)
}

// userItemAttrs builds the full user profile item. Note this carries more
// fields (email, currency, createdAt, entityType) than the EmailIndex GSI
// projects — those extra fields are only ever read via a direct PK/SK
// lookup (Balance today; a future full-profile read tomorrow), never via
// FindByEmail.
func userItemAttrs(u account.User) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":           &types.AttributeValueMemberS{Value: UserPK(u.ID)},
		"SK":           &types.AttributeValueMemberS{Value: ProfileSK()},
		"userId":       &types.AttributeValueMemberS{Value: u.ID},
		"email":        &types.AttributeValueMemberS{Value: u.Email},
		"passwordHash": &types.AttributeValueMemberS{Value: u.PasswordHash},
		"balance":      MarshalMoney(u.Balance),
		"currency":     &types.AttributeValueMemberS{Value: u.Currency},
		"createdAt":    &types.AttributeValueMemberS{Value: u.CreatedAt.Format(time.RFC3339Nano)},
		"GSI1PK":       &types.AttributeValueMemberS{Value: EmailGSI1PK(u.Email)},
		"entityType":   &types.AttributeValueMemberS{Value: entityTypeProfile},
	}
}

// attrString extracts a string (S) attribute from item, reporting false
// when the key is absent or holds a different attribute type.
func attrString(item map[string]types.AttributeValue, key string) (string, bool) {
	av, ok := item[key]
	if !ok {
		return "", false
	}
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return "", false
	}
	return s.Value, true
}

// tableActiveMaxWait bounds how long EnsureTable waits for a freshly
// created table to reach ACTIVE. See the waiter call for why it is minutes
// and not seconds.
const tableActiveMaxWait = 5 * time.Minute

// EnsureTable creates the single table (10/10 provisioned, EmailIndex
// 5/5) if it does not already exist, waits for it to become ACTIVE, and
// enables TTL on expiresAt. Idempotent: ResourceInUseException from a
// concurrent or repeated create is swallowed (design.md's seeding
// contract, shared by docker-compose's one-shot seeder and
// scripts/deploy-aws.sh).
func EnsureTable(ctx context.Context, client *dynamodb.Client, tableName string) error {
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("PK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("SK"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("GSI1PK"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(10),
			WriteCapacityUnits: aws.Int64(10),
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("EmailIndex"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("GSI1PK"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{
					ProjectionType:   types.ProjectionTypeInclude,
					NonKeyAttributes: []string{"userId", "passwordHash", "balance"},
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
	})
	if err != nil {
		var inUse *types.ResourceInUseException
		if errors.As(err, &inUse) {
			return nil
		}
		return fmt.Errorf("dynamo: create table: %w", err)
	}

	// tableActiveMaxWait is generous on purpose. The SDK's TableExists
	// waiter polls on a 20-second minimum delay, so a 30-second budget
	// buys exactly one retry: the waiter aborts as soon as the next
	// scheduled poll would fall outside the window. A real DynamoDB table
	// with a secondary index routinely needs longer than that to reach
	// ACTIVE, so the tight budget failed the very first deployment against
	// AWS while every local run passed — dynamodb-local reports the table
	// ACTIVE immediately, so the wait is a no-op there and the emulator can
	// never surface this.
	//
	// Waiting longer costs nothing when the table is already ACTIVE (the
	// first poll returns and the waiter exits), and the value only has to
	// exceed the slowest realistic creation, not predict it.
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, tableActiveMaxWait); err != nil {
		return fmt.Errorf("dynamo: wait table active: %w", err)
	}

	if _, err := client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("expiresAt"),
			Enabled:       aws.Bool(true),
		},
	}); err != nil {
		return fmt.Errorf("dynamo: enable ttl: %w", err)
	}

	return nil
}
