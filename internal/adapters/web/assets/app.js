import { createApp } from './vendor/vue.esm-browser.prod.js';
import * as api from './api.js';
import { addDays, dayKey, dayOfMonth, dayOfWeek, monthName } from './format.js';
import TopBar from './components/TopBar.js';
import EventCard from './components/EventCard.js';
import BetSlip from './components/BetSlip.js';
import LoginDialog from './components/LoginDialog.js';

/**
 * Root component.
 *
 * State that belongs to the server stays on the server: this holds what the
 * user is looking at (which day range, which card is open, which outcomes
 * are in the slip) and nothing else. Prices, rules and balances are always
 * whatever the last API response said.
 */
const App = {
  components: { TopBar, EventCard, BetSlip, LoginDialog },

  data() {
    return {
      // Catalog
      events: [],
      loading: true,
      loadError: null,
      view: 'agenda',
      /*
       * The calendar anchors on the catalog's own first match day, not on
       * the machine clock. The dataset is the June 2026 World Cup, so a
       * "today" taken from the system date lands outside it entirely and
       * every range filter would answer with an empty list — a demo that
       * looks broken while the API is behaving perfectly. Anchoring here
       * keeps the from/to filters genuinely exercised against real data.
       */
      anchor: null,

      details: {},
      detailLoading: {},
      expanded: {},
      marketIndex: {},

      // Bet slip
      legs: [],
      stake: 10,
      quote: null,
      quoteError: null,
      calculating: false,
      slipOpen: false,
      // Explicit Bet Builder opt-in (spec: bet-slip-calculation/Bet
      // Builder Explicit UI Affordance). isBetBuilder is only ever sent
      // true when this toggle is on — never inferred from picking 2+
      // selections from the same event.
      betBuilder: false,

      // Placement
      placing: false,
      placeResult: null,
      placeError: null,
      probing: false,
      probeResults: null,

      // Session
      token: api.getToken(),
      email: '',
      balanceAmount: null,
      currency: 'PEN',
      loginOpen: false,
      loginPending: false,
      loginError: null,
      // Set when login was opened by an attempted placement, so the bet is
      // placed as soon as the session exists instead of making the user
      // press the button twice.
      placeAfterLogin: false,
    };
  },

  computed: {
    authenticated() {
      return Boolean(this.token);
    },
    selectedIds() {
      return this.legs.map((leg) => leg.selectionId);
    },
    /** Events grouped by local calendar day, in catalog order. */
    groups() {
      const byDay = new Map();
      for (const event of this.events) {
        const key = dayKey(event.startsAt);
        if (!byDay.has(key)) byDay.set(key, []);
        byDay.get(key).push(event);
      }
      return [...byDay.entries()].map(([key, events]) => ({
        key,
        events,
        isAnchor: key === this.anchor,
        dow: dayOfWeek(key),
        dom: dayOfMonth(key),
      }));
    },
    monthLabel() {
      if (!this.groups.length) return '';
      return monthName(this.groups[0].key);
    },
    rangeLabel() {
      if (!this.events.length) return '';
      const first = dayKey(this.events[0].startsAt);
      const last = dayKey(this.events[this.events.length - 1].startsAt);
      if (first === last) return `${dayOfMonth(first)} de ${monthName(first)}`;
      return `${dayOfMonth(first)}–${dayOfMonth(last)} de ${monthName(last)}`;
    },
  },

  watch: {
    /*
     * Any change to the slip or the stake re-prices the whole slip against
     * the API. Debounced so typing an amount does not fire a request per
     * keystroke, and guarded by a request sequence number so a slow earlier
     * response can never overwrite a newer one.
     */
    legs: { handler: 'scheduleCalculate', deep: true },
    stake: 'scheduleCalculate',
    betBuilder: 'scheduleCalculate',
  },

  async mounted() {
    await this.loadEvents();
    if (this.authenticated) await this.refreshBalance();
  },

  methods: {
    // --- catalog ---------------------------------------------------------

    async loadEvents() {
      this.loading = true;
      this.loadError = null;
      try {
        const params = {};
        if (this.view !== 'agenda' && this.anchor) {
          params.from = this.anchor;
          params.to = addDays(this.anchor, this.view === '3d' ? 2 : 6);
        }
        const events = await api.listEvents(params);
        this.events = Array.isArray(events) ? events : [];
        if (!this.anchor && this.events.length) {
          this.anchor = dayKey(this.events[0].startsAt);
        }
        await this.openAnchorDay();
      } catch (err) {
        this.loadError = err;
        this.events = [];
      } finally {
        this.loading = false;
      }
    },

    async setView(view) {
      if (this.view === view) return;
      this.view = view;
      await this.loadEvents();
    },

    /* Design note from the source file: cards on the current day are always
       shown open. */
    async openAnchorDay() {
      const todays = this.events.filter((event) => dayKey(event.startsAt) === this.anchor);
      await Promise.all(todays.map((event) => this.expand(event.id)));
    },

    async toggle(eventId) {
      if (this.expanded[eventId]) {
        this.expanded[eventId] = false;
        return;
      }
      await this.expand(eventId);
    },

    async expand(eventId) {
      this.expanded[eventId] = true;
      if (this.details[eventId] || this.detailLoading[eventId]) return;
      this.detailLoading[eventId] = true;
      try {
        this.details[eventId] = await api.eventDetail(eventId);
      } catch {
        // A detail that cannot be loaded leaves the card open and empty;
        // the list entry itself is still valid and the rest of the calendar
        // must keep working.
        this.details[eventId] = null;
      } finally {
        this.detailLoading[eventId] = false;
      }
    },

    setMarket(eventId, index) {
      this.marketIndex[eventId] = index;
    },

    goToday() {
      const target = document.getElementById(`day-${this.anchor}`);
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    },

    // --- bet slip --------------------------------------------------------

    toggleSelection(leg) {
      const index = this.legs.findIndex((l) => l.selectionId === leg.selectionId);
      if (index >= 0) {
        this.legs.splice(index, 1);
      } else {
        this.legs.push(leg);
      }
      this.placeResult = null;
      this.placeError = null;
      this.probeResults = null;
    },

    removeLeg(selectionId) {
      this.legs = this.legs.filter((leg) => leg.selectionId !== selectionId);
    },

    clearSlip() {
      this.legs = [];
      this.quote = null;
      this.quoteError = null;
      this.placeResult = null;
      this.placeError = null;
      this.probeResults = null;
    },

    scheduleCalculate() {
      clearTimeout(this._calcTimer);
      this._calcTimer = setTimeout(() => this.calculate(), 250);
    },

    async calculate() {
      if (!this.legs.length) {
        this.quote = null;
        this.quoteError = null;
        return;
      }
      const amount = Number(this.stake);
      if (!Number.isFinite(amount) || amount <= 0) {
        this.quoteError = null;
        return;
      }

      const seq = (this._calcSeq = (this._calcSeq || 0) + 1);
      this.calculating = true;
      try {
        const quote = await api.calculate({
          selectionIds: this.selectedIds,
          stake: amount,
          isBetBuilder: this.betBuilder,
        });
        if (seq !== this._calcSeq) return;
        this.quote = quote;
        this.quoteError = null;
        this.currency = quote.currency || this.currency;
      } catch (err) {
        if (seq !== this._calcSeq) return;
        this.quoteError = err;
        this.quote = null;
      } finally {
        if (seq === this._calcSeq) this.calculating = false;
      }
    },

    // --- placement -------------------------------------------------------

    async placeBet() {
      if (!this.authenticated) {
        this.placeAfterLogin = true;
        this.loginOpen = true;
        return;
      }
      this.placing = true;
      this.placeError = null;
      this.placeResult = null;
      try {
        const result = await api.place({
          selectionIds: this.selectedIds,
          stake: Number(this.stake),
          idempotencyKey: api.newIdempotencyKey(),
          isBetBuilder: this.betBuilder,
        });
        this.placeResult = result;
        if (result.balanceAfter !== null && result.balanceAfter !== undefined) {
          this.balanceAmount = result.balanceAfter;
        } else {
          await this.refreshBalance();
        }
        this.legs = [];
        this.quote = null;
      } catch (err) {
        this.placeError = err;
        if (err.status === 401) this.signOut();
      } finally {
        this.placing = false;
      }
    },

    /**
     * Fires two placements at the same instant, each with its own
     * idempotency key so neither is deduplicated as a retry of the other.
     *
     * This is the balance-under-concurrency requirement made visible: with
     * a balance that funds only one of them, the API must accept exactly
     * one and reject the other, and the resulting balance must equal
     * exactly one debit.
     */
    async probeConcurrency() {
      this.probing = true;
      this.probeResults = null;
      this.placeError = null;
      this.placeResult = null;

      const payload = { selectionIds: this.selectedIds, stake: Number(this.stake), isBetBuilder: this.betBuilder };
      const attempts = [
        api.place({ ...payload, idempotencyKey: api.newIdempotencyKey() }),
        api.place({ ...payload, idempotencyKey: api.newIdempotencyKey() }),
      ];

      const settled = await Promise.allSettled(attempts);
      this.probeResults = settled.map((outcome) => {
        if (outcome.status === 'fulfilled') {
          const bet = outcome.value;
          return {
            accepted: bet.status === 'accepted',
            label: `${bet.status} · ${bet.type} · bet ${bet.betId.slice(0, 10)}…`,
          };
        }
        const err = outcome.reason;
        return { accepted: false, label: `${err.code} · ${err.message}` };
      });

      await this.refreshBalance();
      this.probing = false;
    },

    // --- session ---------------------------------------------------------

    async doLogin({ email, password }) {
      this.loginPending = true;
      this.loginError = null;
      try {
        const { token } = await api.login({ email, password });
        api.setToken(token);
        this.token = token;
        this.email = email;
        this.loginOpen = false;
        await this.refreshBalance();
        if (this.placeAfterLogin) {
          this.placeAfterLogin = false;
          await this.placeBet();
        }
      } catch (err) {
        this.loginError = err;
      } finally {
        this.loginPending = false;
      }
    },

    async refreshBalance() {
      try {
        const result = await api.balance();
        this.balanceAmount = result.balance;
        this.currency = result.currency || this.currency;
      } catch (err) {
        // An expired or rejected token must not leave the header showing a
        // stale balance as if the session were still alive.
        if (err.status === 401) this.signOut();
      }
    },

    signOut() {
      api.setToken('');
      this.token = '';
      this.email = '';
      this.balanceAmount = null;
    },

    openLogin() {
      this.placeAfterLogin = false;
      this.loginError = null;
      this.loginOpen = true;
    },
  },

  template: `
    <div class="shell">
      <TopBar
        :authenticated="authenticated"
        :balance="balanceAmount"
        :currency="currency"
        :email="email"
        @login="openLogin"
        @logout="signOut"
      />

      <div class="tabs-wrap">
        <div class="tabs">
          <div class="tabs-brand">fifa 2026<br>world cup</div>
          <!--
            Drawn icons rather than the box-drawing characters that stand in
            for them in a design tool: those render as solid blocks, or not
            at all, depending on the system font. currentColor makes each one
            follow its tab's active state for free.
          -->
          <button class="tab" :class="{ 'is-active': view === 'agenda' }" type="button" @click="setView('agenda')">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <circle cx="2.5" cy="4" r="1.3" fill="currentColor"/>
              <circle cx="2.5" cy="8" r="1.3" fill="currentColor"/>
              <circle cx="2.5" cy="12" r="1.3" fill="currentColor"/>
              <path d="M6 4h8M6 8h8M6 12h5" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
            </svg>
            <span>Agenda</span>
          </button>
          <button class="tab" :class="{ 'is-active': view === '3d' }" type="button" @click="setView('3d')">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="1.3" y="2.7" width="13.4" height="10.6" rx="1.8" stroke="currentColor" stroke-width="1.5"/>
              <path d="M5.8 2.7v10.6M10.2 2.7v10.6" stroke="currentColor" stroke-width="1.5"/>
            </svg>
            <span>3 Días</span>
          </button>
          <button class="tab" :class="{ 'is-active': view === 'week' }" type="button" @click="setView('week')">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <rect x="1.5" y="3.2" width="13" height="11" rx="1.8" stroke="currentColor" stroke-width="1.5"/>
              <path d="M1.5 6.6h13" stroke="currentColor" stroke-width="1.5"/>
              <path d="M5 1.6v2.6M11 1.6v2.6" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
            </svg>
            <span>Semana</span>
          </button>
        </div>
      </div>

      <div class="month-bar">
        <span class="month-title">{{ monthLabel }}</span>
        <button class="chip-today" type="button" @click="goToday">Hoy</button>
      </div>
      <p class="catalog-note" v-if="rangeLabel">
        Catálogo Mundial 2026 · {{ rangeLabel }}. El calendario se ancla al primer día con partidos.
      </p>

      <main class="feed">
        <template v-if="loading">
          <div style="padding:12px">
            <div class="skeleton-card" v-for="n in 4" :key="n">
              <div class="skeleton-line" style="width:65%"></div>
              <div class="skeleton-line" style="width:40%"></div>
            </div>
          </div>
        </template>

        <div v-else-if="loadError" style="padding:16px">
          <div class="api-error">
            <span class="api-error-code">{{ loadError.code }}</span>
            <span>{{ loadError.message }}</span>
          </div>
        </div>

        <p v-else-if="!events.length" class="empty">No hay eventos en este rango.</p>

        <section
          v-else
          v-for="group in groups"
          :key="group.key"
          :id="'day-' + group.key"
          class="day-group"
          :class="{ 'is-today': group.isAnchor }"
        >
          <div class="day-rail">
            <div class="day-rail-inner">
              <div class="day-dow">{{ group.dow }}</div>
              <div class="day-dom">{{ group.dom }}</div>
            </div>
          </div>
          <div class="day-events">
            <EventCard
              v-for="event in group.events"
              :key="event.id"
              :event="event"
              :detail="details[event.id] || null"
              :expanded="!!expanded[event.id]"
              :loading="!!detailLoading[event.id]"
              :market-index="marketIndex[event.id] || 0"
              :selected-ids="selectedIds"
              @toggle="toggle(event.id)"
              @market="setMarket(event.id, $event)"
              @select="toggleSelection"
            />
          </div>
        </section>
      </main>

      <!--
        Hidden while the sheet is open: the button opens a panel that is
        already on screen, so leaving it visible beside the panel only reads
        as a stray control.
      -->
      <div class="fab-layer" v-if="!slipOpen">
        <div class="fab-rail">
          <button class="fab" type="button" @click="slipOpen = true" aria-label="Abrir cupón">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M6 3h9l4 4v14H6z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/>
              <path d="M14 3v5h5M9 13h7M9 17h5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
            </svg>
            <span class="fab-label">Cupón</span>
            <span class="fab-count" v-if="legs.length">{{ legs.length }}</span>
          </button>
        </div>
      </div>

      <BetSlip
        v-if="slipOpen"
        :legs="legs"
        :stake="stake"
        :quote="quote"
        :quote-error="quoteError"
        :calculating="calculating"
        :placing="placing"
        :place-result="placeResult"
        :place-error="placeError"
        :authenticated="authenticated"
        :currency="currency"
        :probing="probing"
        :probe-results="probeResults"
        :bet-builder="betBuilder"
        @close="slipOpen = false"
        @remove="removeLeg"
        @clear="clearSlip"
        @update:stake="stake = $event"
        @update:bet-builder="betBuilder = $event"
        @place="placeBet"
        @probe="probeConcurrency"
      />

      <LoginDialog
        v-if="loginOpen"
        :pending="loginPending"
        :error="loginError"
        @close="loginOpen = false"
        @submit="doLogin"
      />
    </div>
  `,
};

createApp(App).mount('#app');
