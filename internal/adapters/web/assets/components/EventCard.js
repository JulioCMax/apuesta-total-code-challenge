import { odds as fmtOdds, time as fmtTime, teamCode, isUndefinedRival } from '../format.js';

/**
 * One fixture in the calendar, collapsed or expanded.
 *
 * The card never fetches anything and never decides what a market is: it
 * renders the event summary from GET /events and, once expanded, the detail
 * from GET /events/:id exactly in the order the API returned it. The
 * default market order (1X2, Total de goles, Ambos equipos anotan, Primer
 * gol) is the server's contract, so the carousel is a plain index over
 * `detail.markets` with no client-side sorting.
 */
export default {
  name: 'EventCard',
  props: {
    event: { type: Object, required: true },
    detail: { type: Object, default: null },
    expanded: { type: Boolean, default: false },
    loading: { type: Boolean, default: false },
    marketIndex: { type: Number, default: 0 },
    selectedIds: { type: Array, default: () => [] },
  },
  emits: ['toggle', 'market', 'select'],
  computed: {
    homeUndefined() {
      return isUndefinedRival(this.event.home);
    },
    awayUndefined() {
      return isUndefinedRival(this.event.away);
    },
    homeCode() {
      return teamCode(this.event.home);
    },
    awayCode() {
      return teamCode(this.event.away);
    },
    kickoff() {
      return fmtTime(this.event.startsAt);
    },
    markets() {
      return (this.detail && this.detail.markets) || [];
    },
    market() {
      if (!this.markets.length) return null;
      const index = Math.min(this.marketIndex, this.markets.length - 1);
      return this.markets[index];
    },
    activeIndex() {
      if (!this.markets.length) return 0;
      return Math.min(this.marketIndex, this.markets.length - 1);
    },
    /*
     * Read from the list entry, not from the detail. GET /events carries
     * the UI metadata for every event, which is what lets the badges render
     * on a collapsed card without fetching that event's detail first. The
     * detail is only a fallback and returns the very same object.
     */
    settings() {
      return this.event.settings || (this.detail && this.detail.settings) || null;
    },
    /* A suspended event is still listed — the catalog says so — but every
       outcome in it is unbettable, which the API would reject anyway. */
    suspended() {
      return Boolean(this.event.isSuspended);
    },
  },
  methods: {
    fmtOdds,
    isSelected(selectionId) {
      return this.selectedIds.includes(selectionId);
    },
    selectionLabel(selection) {
      return selection.line ? `${selection.name} ${selection.line}` : selection.name;
    },
    columnsClass(count) {
      if (count === 2) return 'odds-row cols-2';
      if (count >= 4) return 'odds-row cols-4';
      return 'odds-row';
    },
    onSelect(market, selection) {
      if (selection.isDisabled || this.suspended) return;
      this.$emit('select', {
        selectionId: selection.id,
        eventId: this.event.id,
        eventName: this.event.name,
        marketName: market.name,
        selectionName: this.selectionLabel(selection),
        odds: selection.odds,
      });
    },
  },
  template: `
    <article class="event-card">
      <div v-if="event.isLive" class="stream-banner">
        <span aria-hidden="true">▶</span>
        <span>Mira aquí la transmisión en vivo</span>
      </div>

      <button
        class="event-head"
        type="button"
        :aria-expanded="String(expanded)"
        @click="$emit('toggle')"
      >
        <div class="event-name">
          <template v-if="homeUndefined">
            <span class="undefined-rival">{{ event.home }}</span>
          </template>
          <template v-else>{{ event.home }}</template>
          <span> vs </span>
          <template v-if="awayUndefined">
            <span class="undefined-rival">{{ event.away }}</span>
          </template>
          <template v-else>{{ event.away }}</template>
        </div>

        <div class="event-group" v-if="event.group">Grupo {{ event.group }}</div>
        <div class="event-group" v-else>{{ event.phase }}</div>

        <div class="event-meta">
          <span v-if="event.isLive" class="live-badge">EN VIVO</span>
          <span v-else class="event-time">{{ kickoff }}</span>
          <span class="globe" aria-hidden="true">⊕</span>
          <span class="event-league">{{ event.league }}</span>

          <span class="flags">
            <!-- A drawn icon, not a text glyph: the block characters that
                 read as a bar chart in a design tool render as a solid
                 square in most system fonts. -->
            <span v-if="settings && settings.hasStatistics" class="badge badge-stats" title="Estadísticas disponibles">
              <svg width="10" height="10" viewBox="0 0 12 12" aria-hidden="true">
                <rect x="0" y="6" width="3" height="6" rx="1" fill="currentColor"/>
                <rect x="4.5" y="3" width="3" height="9" rx="1" fill="currentColor"/>
                <rect x="9" y="0" width="3" height="12" rx="1" fill="currentColor"/>
              </svg>
            </span>
            <span v-if="settings && settings.isBetBuilderEnabled" class="badge badge-bb" title="BetBuilder habilitado">BB</span>
            <span class="flag-chip">{{ homeCode }}</span>
            <span class="flag-chip">{{ awayCode }}</span>
          </span>
        </div>

        <svg class="chevron" :class="{ 'is-open': expanded }" width="16" height="16" viewBox="0 0 20 20" fill="none" aria-hidden="true">
          <path d="M5 7.5 10 12.5 15 7.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </button>

      <div v-if="expanded" class="event-body">
        <div v-if="loading" class="skeleton-card" style="border:0;padding:0;margin:0">
          <div class="skeleton-line" style="width:60%"></div>
          <div class="skeleton-line" style="width:45%"></div>
          <div class="skeleton-line" style="height:34px"></div>
        </div>

        <template v-else-if="market">
          <div class="team-row">
            <span class="flag-chip">{{ homeCode }}</span>
            <span>{{ event.home }}</span>
          </div>
          <div class="team-row">
            <span class="flag-chip">{{ awayCode }}</span>
            <span>{{ event.away }}</span>
          </div>

          <div class="market-title">
            <span>{{ market.name }}</span>
            <!-- MarketType id, verbatim from the API: the identifier the UI
                 keys special treatments (Super Cuota) off. -->
            <span class="badge badge-sc" :title="'MarketType._id: ' + market.marketType.id">
              {{ market.marketType.id }}
            </span>
          </div>

          <div :class="columnsClass(market.selections.length)">
            <button
              v-for="selection in market.selections"
              :key="selection.id"
              type="button"
              class="odd"
              :class="{ 'is-selected': isSelected(selection.id) }"
              :disabled="selection.isDisabled || suspended"
              @click="onSelect(market, selection)"
            >
              <!--
                Super Cuota: the API only ever sends originalOdds for a
                curated, boosted selection (never null — omitted when
                absent), so its mere presence is the whole signal. The
                struck-through original sits right beside the boosted
                value so the improvement reads as honest, not as a
                second, unexplained number.
              -->
              <span v-if="selection.originalOdds != null" class="badge badge-boost" title="Super Cuota: cuota mejorada">SC</span>
              <span class="odd-value-row">
                <span class="odd-value">{{ fmtOdds(selection.odds) }}</span>
                <span v-if="selection.originalOdds != null" class="odd-original" :title="'Cuota original: ' + fmtOdds(selection.originalOdds)">
                  {{ fmtOdds(selection.originalOdds) }}
                </span>
              </span>
              <span class="odd-label">{{ selectionLabel(selection) }}</span>
            </button>
          </div>

          <div class="dots" v-if="markets.length > 1">
            <button
              v-for="(m, i) in markets"
              :key="m.id"
              type="button"
              class="dot"
              :class="{ 'is-active': i === activeIndex }"
              :title="m.name"
              :aria-label="m.name"
              @click="$emit('market', i)"
            ></button>
          </div>
        </template>

        <p v-else class="empty" style="padding:14px 0">
          Este evento no tiene mercados disponibles.
        </p>
      </div>
    </article>
  `,
};
