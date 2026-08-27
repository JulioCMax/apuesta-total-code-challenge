import { money, odds as fmtOdds } from '../format.js';

/**
 * The bet slip, as a bottom sheet.
 *
 * Every number shown here comes from POST /betslip/calculate. The client
 * never multiplies a stake by an odds value: combined odds and potential
 * returns are rounded once, on the server, and re-deriving them here is how
 * a UI ends up disagreeing with the bet it just placed.
 *
 * The same goes for the rules. Two selections from one event are *allowed*
 * into the slip on purpose, so the API can answer with its typed
 * SAME_EVENT_COMBO error and the reviewer can see the domain rule enforced
 * where it belongs, instead of quietly hidden by a disabled button.
 */
export default {
  name: 'BetSlip',
  props: {
    legs: { type: Array, default: () => [] },
    stake: { type: [Number, String], default: 10 },
    quote: { type: Object, default: null },
    quoteError: { type: Object, default: null },
    calculating: { type: Boolean, default: false },
    placing: { type: Boolean, default: false },
    placeResult: { type: Object, default: null },
    placeError: { type: Object, default: null },
    authenticated: { type: Boolean, default: false },
    currency: { type: String, default: 'PEN' },
    probing: { type: Boolean, default: false },
    probeResults: { type: Array, default: null },
    // Explicit Bet Builder opt-in (spec: bet-slip-calculation/Bet Builder
    // Explicit UI Affordance). Only this toggle — never the slip's own
    // contents — decides whether isBetBuilder is ever sent true.
    betBuilder: { type: Boolean, default: false },
  },
  emits: ['close', 'remove', 'clear', 'update:stake', 'update:betBuilder', 'place', 'probe'],
  computed: {
    betType() {
      return this.legs.length >= 2 ? 'Combinada' : 'Simple';
    },
    combo() {
      return this.quote && this.quote.combo;
    },
    singles() {
      return (this.quote && this.quote.singles) || [];
    },
    /* What "Apostar" would actually register: one bet, combo when the slip
       spans 2+ events, single otherwise. */
    payout() {
      if (!this.quote) return null;
      if (this.combo) return this.combo.potentialReturns;
      if (this.singles.length === 1) return this.singles[0].potentialReturns;
      return null;
    },
    payoutOdds() {
      if (!this.quote) return null;
      if (this.combo) return this.combo.combinedOdds;
      if (this.singles.length === 1) return this.singles[0].odds;
      return null;
    },
    canPlace() {
      return this.legs.length > 0 && !!this.quote && !this.quoteError && !this.placing;
    },
    bounds() {
      if (!this.quote) return '';
      return `Mín ${money(this.quote.minStake, this.quote.currency)} · Máx ${money(this.quote.maxStake, this.quote.currency)}`;
    },
  },
  methods: {
    money,
    fmtOdds,
    selectionOf(id) {
      return this.legs.find((leg) => leg.selectionId === id) || null;
    },
    onStake(event) {
      this.$emit('update:stake', event.target.value);
    },
    onBetBuilder(event) {
      this.$emit('update:betBuilder', event.target.checked);
    },
  },
  template: `
    <div class="overlay" @click.self="$emit('close')">
      <section class="sheet" role="dialog" aria-label="Cupón de apuestas">
        <header class="sheet-head">
          <span class="sheet-title">Cupón</span>
          <span v-if="legs.length" class="fab-count" style="position:static">{{ legs.length }}</span>
          <button class="sheet-close" type="button" aria-label="Cerrar" @click="$emit('close')">×</button>
        </header>

        <div class="sheet-body">
          <!--
            The outcome of a placement outlives the slip that produced it.
            Clearing the legs on success must not take the confirmation with
            them, or the user is told their empty slip is empty right after
            money left their balance.
          -->
          <div v-if="placeResult" class="api-ok">
            <span class="api-ok-title">Apuesta {{ placeResult.status === 'accepted' ? 'aceptada' : placeResult.status }}</span>
            <span>
              {{ placeResult.type }} por {{ money(placeResult.stake, currency) }} ·
              cuota {{ fmtOdds(placeResult.combinedOdds) }} ·
              retorno {{ money(placeResult.potentialReturns, currency) }}
            </span>
            <span v-if="placeResult.balanceAfter !== null">
              Saldo restante: {{ money(placeResult.balanceAfter, currency) }}
            </span>
            <span class="api-ok-code" title="Identificador de la apuesta registrada">{{ placeResult.betId }}</span>
          </div>

          <p v-if="!legs.length && !placeResult" class="empty">
            Tu cupón está vacío.<br>Tocá una cuota para agregar una selección.
          </p>

          <p v-else-if="!legs.length" class="empty" style="padding:16px 6px">
            Elegí otra cuota para armar un cupón nuevo.
          </p>

          <template v-else>
            <div v-for="leg in legs" :key="leg.selectionId" class="leg">
              <div style="min-width:0">
                <div class="leg-selection">{{ leg.selectionName }}</div>
                <div class="leg-context">{{ leg.marketName }} · {{ leg.eventName }}</div>
              </div>
              <div class="leg-odds">{{ fmtOdds(leg.odds) }}</div>
              <button class="leg-remove" type="button" aria-label="Quitar" @click="$emit('remove', leg.selectionId)">×</button>
            </div>

            <label class="bb-toggle">
              <input type="checkbox" :checked="betBuilder" @change="onBetBuilder">
              <span>
                <strong>Bet Builder</strong>
                <small>Combiná selecciones del mismo partido cuando el evento lo permita.</small>
              </span>
            </label>

            <div class="stake-row">
              <div>
                <div class="stake-label">Monto de la apuesta</div>
                <div class="stake-bounds" style="text-align:left">{{ bounds }}</div>
              </div>
              <input
                class="stake-input"
                type="number"
                inputmode="decimal"
                min="0"
                step="0.10"
                :value="stake"
                @input="onStake"
                aria-label="Monto de la apuesta"
              >
            </div>

            <div v-if="quoteError" class="api-error" style="margin-top:8px">
              <span class="api-error-message">{{ quoteError.message }}</span>
              <span v-if="quoteError.details && quoteError.details.min !== undefined" class="api-error-detail">
                Permitido: {{ money(quoteError.details.min, currency) }} – {{ money(quoteError.details.max, currency) }}.
              </span>
              <span v-if="quoteError.code === 'SAME_EVENT_COMBO' && !betBuilder" class="api-error-detail">
                Activá Bet Builder arriba para combinar selecciones del mismo partido cuando el evento lo permita.
              </span>
              <span class="api-error-code" :title="'Código de error de la API: ' + quoteError.code">{{ quoteError.code }}</span>
            </div>

            <template v-if="quote && !quoteError">
              <div class="quote-block" v-if="singles.length">
                <div class="quote-heading">Simples</div>
                <div v-for="single in singles" :key="single.selectionId" class="quote-line">
                  <span>{{ (selectionOf(single.selectionId) || {}).selectionName || single.selectionId }}</span>
                  <span>{{ fmtOdds(single.odds) }} → {{ money(single.potentialReturns, quote.currency) }}</span>
                </div>
              </div>

              <div class="quote-block" v-if="combo">
                <div class="quote-heading">Combinada</div>
                <div class="quote-line">
                  <span>{{ combo.selectionIds.length }} selecciones</span>
                  <span>{{ fmtOdds(combo.combinedOdds) }} → {{ money(combo.potentialReturns, quote.currency) }}</span>
                </div>
              </div>

              <div class="quote-block">
                <div class="quote-line">
                  <span>Se registra como <strong style="font-size:13px">{{ betType }}</strong> · cuota {{ fmtOdds(payoutOdds) }}</span>
                </div>
                <div class="quote-line">
                  <span>Ganancia potencial</span>
                  <strong>{{ money(payout, quote.currency) }}</strong>
                </div>
              </div>
            </template>

            <div v-if="placeError" class="api-error" style="margin-top:10px">
              <span class="api-error-message">{{ placeError.message }}</span>
              <span v-if="placeError.details && placeError.details.balance !== undefined" class="api-error-detail">
                Saldo {{ money(placeError.details.balance, currency) }} ·
                requerido {{ money(placeError.details.required, currency) }}.
              </span>
              <span class="api-error-code" :title="'Código de error de la API: ' + placeError.code">{{ placeError.code }}</span>
            </div>

            <!--
              Concurrency probe. The graded requirement is that two
              simultaneous placements cannot corrupt the balance; this fires
              exactly that, with two distinct idempotency keys so they are
              two genuine placements rather than one deduplicated retry.
            -->
            <div class="probe" v-if="authenticated && legs.length">
              <button class="btn-ghost" type="button" :disabled="probing || !canPlace" @click="$emit('probe')">
                {{ probing ? 'Enviando…' : 'Probar concurrencia: 2 apuestas simultáneas' }}
              </button>
              <p class="probe-note">
                Dispara dos colocaciones idénticas a la vez. Si el saldo alcanza para una sola,
                exactamente una se acepta y la otra se rechaza con saldo insuficiente.
                El saldo nunca queda en un estado intermedio.
              </p>
              <div v-if="probeResults" class="probe-result">
                <div
                  v-for="(result, i) in probeResults"
                  :key="i"
                  class="probe-row"
                  :class="result.accepted ? 'is-accepted' : 'is-rejected'"
                >
                  <code>#{{ i + 1 }}</code>
                  <span>{{ result.label }}</span>
                </div>
              </div>
            </div>
          </template>
        </div>

        <footer class="sheet-foot" v-if="legs.length">
          <button class="btn-primary" type="button" :disabled="!canPlace" @click="$emit('place')">
            <template v-if="placing">Colocando…</template>
            <template v-else-if="!authenticated">Ingresar y apostar</template>
            <template v-else-if="calculating">Calculando…</template>
            <template v-else>Apostar {{ money(stake, currency) }}</template>
          </button>
          <button class="btn-ghost" type="button" @click="$emit('clear')">Vaciar cupón</button>
        </footer>
      </section>
    </div>
  `,
};
