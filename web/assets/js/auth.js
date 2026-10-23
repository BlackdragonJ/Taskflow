// Login, registro y recuperación de contraseña.

const EMAIL_RE = /^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$/;

// Mismas reglas que auth.PasswordProblems en el backend
const PW_RULES = [
  { key: 'len', text: 'Mínimo 8 caracteres', test: (v) => v.length >= 8 },
  { key: 'upper', text: 'Una mayúscula', test: (v) => /[A-ZÁÉÍÓÚÑ]/.test(v) },
  { key: 'lower', text: 'Una minúscula', test: (v) => /[a-záéíóúñ]/.test(v) },
  { key: 'digit', text: 'Un número', test: (v) => /\d/.test(v) },
  { key: 'special', text: 'Un carácter especial', test: (v) => /[^A-Za-z0-9\sÁÉÍÓÚáéíóúÑñ]/.test(v) },
];

function bindPasswordRules($input, $list) {
  $list.html(PW_RULES.map((r) =>
    `<li data-rule="${r.key}"><i class="bi bi-circle me-1"></i>${r.text}</li>`).join(''));
  $input.on('input', () => {
    const v = $input.val();
    PW_RULES.forEach((r) => {
      const ok = r.test(v);
      $list.find(`[data-rule="${r.key}"]`).toggleClass('ok', ok)
        .find('.bi').toggleClass('bi-circle', !ok).toggleClass('bi-check-circle-fill', ok);
    });
  });
}

function passwordValid(v) {
  return PW_RULES.every((r) => r.test(v));
}

// Validación del lado del cliente; el servidor vuelve a validar todo.
function validate($form, rules) {
  const errors = {};
  Object.entries(rules).forEach(([name, check]) => {
    const val = ($form.find(`[name="${name}"]`).val() || '').trim();
    const msg = check(val);
    if (msg) errors[name] = msg;
  });
  if (Object.keys(errors).length) {
    UI.fieldErrors($form, errors);
    return false;
  }
  UI.clearFieldErrors($form);
  return true;
}

const required = (label) => (v) => (v ? '' : `${label} es obligatorio`);
const emailRule = (v) => {
  if (!v) return 'El correo es obligatorio';
  return EMAIL_RE.test(v) ? '' : 'El formato del correo no es válido';
};

function initLogin() {
  if (Session.token()) {
    window.location.replace('/dashboard');
    return;
  }
  const params = new URLSearchParams(window.location.search);
  if (params.get('expired')) UI.toast('Tu sesión expiró, inicia sesión de nuevo', 'warning');
  if (params.get('registered')) UI.toast('Cuenta creada, ya puedes iniciar sesión');
  if (params.get('reset')) UI.toast('Contraseña actualizada');

  const $form = $('#loginForm');
  $form.on('submit', async (e) => {
    e.preventDefault();
    if (!validate($form, { email: emailRule, password: required('La contraseña') })) return;

    const $btn = $form.find('[type=submit]');
    UI.setLoading($btn, true);
    try {
      const res = await Api.post('/auth/login', UI.formData($form));
      Session.save(res.token, res.user);
      window.location.href = '/dashboard';
    } catch (err) {
      UI.showError(err, $form);
      UI.setLoading($btn, false);
    }
  });
}

function initRegister() {
  const $form = $('#registerForm');
  bindPasswordRules($form.find('[name=password]'), $('#pwRules'));

  $form.on('submit', async (e) => {
    e.preventDefault();
    const ok = validate($form, {
      name: required('El nombre'),
      email: emailRule,
      password: (v) => (passwordValid(v) ? '' : 'La contraseña no cumple los requisitos'),
      password2: (v) => (v === $form.find('[name=password]').val() ? '' : 'Las contraseñas no coinciden'),
    });
    if (!ok) return;

    const $btn = $form.find('[type=submit]');
    UI.setLoading($btn, true);
    try {
      const { name, email, password } = UI.formData($form);
      await Api.post('/auth/register', { name, email, password });
      window.location.href = '/?registered=1';
    } catch (err) {
      UI.showError(err, $form);
      UI.setLoading($btn, false);
    }
  });
}

function initForgot() {
  const $req = $('#forgotForm');
  const $reset = $('#resetForm');
  bindPasswordRules($reset.find('[name=password]'), $('#pwRules'));

  $req.on('submit', async (e) => {
    e.preventDefault();
    if (!validate($req, { email: emailRule })) return;
    const $btn = $req.find('[type=submit]');
    UI.setLoading($btn, true);
    try {
      const res = await Api.post('/auth/forgot', UI.formData($req));
      UI.toast(res.message, 'info');
      // en desarrollo el backend devuelve el código para poder probar sin correo
      if (res.dev_token) {
        $reset.find('[name=token]').val(res.dev_token);
        $('#devHint').removeClass('d-none');
      }
      $('#step1').addClass('d-none');
      $('#step2').removeClass('d-none');
    } catch (err) {
      UI.showError(err, $req);
    } finally {
      UI.setLoading($btn, false);
    }
  });

  $reset.on('submit', async (e) => {
    e.preventDefault();
    const ok = validate($reset, {
      token: required('El código'),
      password: (v) => (passwordValid(v) ? '' : 'La contraseña no cumple los requisitos'),
      password2: (v) => (v === $reset.find('[name=password]').val() ? '' : 'Las contraseñas no coinciden'),
    });
    if (!ok) return;
    const $btn = $reset.find('[type=submit]');
    UI.setLoading($btn, true);
    try {
      const { token, password } = UI.formData($reset);
      await Api.post('/auth/reset', { token, password });
      window.location.href = '/?reset=1';
    } catch (err) {
      UI.showError(err, $reset);
      UI.setLoading($btn, false);
    }
  });
}
