import { money } from '../format.js';

/**
 * Header bar: brand, balance and session affordance.
 *
 * "Recargar" and the menu button are rendered because the source design
 * has them, but they are inert: topping up an account and account
 * management are outside the API's scope, and wiring them to nothing that
 * exists would be worse than leaving them plainly decorative.
 */
export default {
  name: 'TopBar',
  props: {
    authenticated: { type: Boolean, default: false },
    balance: { type: Number, default: null },
    currency: { type: String, default: 'PEN' },
    email: { type: String, default: '' },
  },
  emits: ['login', 'logout'],
  computed: {
    initial() {
      return (this.email || 'A').trim().charAt(0) || 'A';
    },
    amount() {
      return this.balance === null ? '—' : money(this.balance, this.currency);
    },
    bonus() {
      return money(0, this.currency);
    },
  },
  template: `
    <header class="topbar">
      <div class="logo" aria-label="Apuesta Total">at</div>

      <template v-if="authenticated">
        <div class="topbar-balance">
          <div class="topbar-amount">{{ amount }}</div>
          <div class="topbar-bonus">Bono {{ bonus }}</div>
        </div>
        <button
          class="avatar"
          type="button"
          :title="email + ' — cerrar sesión'"
          @click="$emit('logout')"
        >{{ initial }}</button>
        <button class="btn-recharge" type="button" disabled title="Fuera del alcance de la API">
          Recargar
        </button>
      </template>

      <template v-else>
        <div class="topbar-balance"></div>
        <button class="btn-login" type="button" @click="$emit('login')">Ingresar</button>
      </template>

      <button class="burger" type="button" aria-label="Menú" disabled>
        <span></span><span></span><span></span>
      </button>
    </header>
  `,
};
