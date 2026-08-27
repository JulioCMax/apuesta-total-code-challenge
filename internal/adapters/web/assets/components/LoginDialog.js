/**
 * Login dialog.
 *
 * The demo credentials are prefilled and stated in plain sight. They are
 * the fixed seed accounts (cmd/seed) of a throwaway local table, so
 * treating them as a secret would only make the demo harder to run without
 * protecting anything.
 */
export default {
  name: 'LoginDialog',
  props: {
    pending: { type: Boolean, default: false },
    error: { type: Object, default: null },
  },
  emits: ['close', 'submit'],
  data() {
    return {
      email: 'demo1@apuestatotal.com',
      password: 'Demo1234!',
    };
  },
  methods: {
    submit() {
      if (this.pending) return;
      this.$emit('submit', { email: this.email.trim(), password: this.password });
    },
  },
  template: `
    <div class="overlay" style="align-items:center" @click.self="$emit('close')">
      <form class="dialog" role="dialog" aria-label="Iniciar sesión" @submit.prevent="submit">
        <h2>Iniciar sesión</h2>

        <div class="field">
          <label for="login-email">Correo</label>
          <input id="login-email" v-model="email" type="email" autocomplete="username" required>
        </div>

        <div class="field">
          <label for="login-password">Contraseña</label>
          <input id="login-password" v-model="password" type="password" autocomplete="current-password" required>
        </div>

        <div v-if="error" class="api-error">
          <span class="api-error-code">{{ error.code }}</span>
          <span>{{ error.message }}</span>
        </div>

        <button class="btn-primary" type="submit" :disabled="pending">
          {{ pending ? 'Ingresando…' : 'Ingresar' }}
        </button>
        <button class="btn-ghost" type="button" @click="$emit('close')">Cancelar</button>

        <p class="dialog-hint">
          Cuentas sembradas para la demostración:
          <strong>demo1@apuestatotal.com</strong> y <strong>demo2@apuestatotal.com</strong>,
          ambas con la contraseña <strong>Demo1234!</strong>.
        </p>
      </form>
    </div>
  `,
};
