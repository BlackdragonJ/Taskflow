// Cliente AJAX + utilidades de UI compartidas por todas las páginas.

const Session = (() => {
  const TOKEN_KEY = 'tf_token';
  const USER_KEY = 'tf_user';

  // localStorage puede fallar en modo privado, por eso el try/catch
  function read(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }
  function write(key, val) {
    try {
      if (val === null) localStorage.removeItem(key);
      else localStorage.setItem(key, val);
    } catch (e) { /* sin storage no hay sesión persistente */ }
  }

  return {
    token: () => read(TOKEN_KEY),
    user() {
      try { return JSON.parse(read(USER_KEY)) || null; } catch (e) { return null; }
    },
    save(token, user) {
      write(TOKEN_KEY, token);
      write(USER_KEY, JSON.stringify(user));
    },
    setUser(user) { write(USER_KEY, JSON.stringify(user)); },
    clear() { write(TOKEN_KEY, null); write(USER_KEY, null); },
    isAdmin() { const u = this.user(); return !!u && u.role === 'admin'; },
  };
})();

class ApiError extends Error {
  constructor(status, code, message, fields) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields || {};
  }
}

const Api = (() => {
  function parseError(xhr) {
    if (xhr.status === 0) {
      return new ApiError(0, 'network', 'No hay conexión con el servidor');
    }
    const body = xhr.responseJSON;
    if (body && body.error) {
      return new ApiError(xhr.status, body.error.code, body.error.message, body.error.fields);
    }
    return new ApiError(xhr.status, 'unknown', `Error inesperado (${xhr.status})`);
  }

  function request(method, url, data) {
    return new Promise((resolve, reject) => {
      const headers = {};
      const token = Session.token();
      if (token) headers.Authorization = 'Bearer ' + token;

      $.ajax({
        url: '/api' + url,
        method,
        headers,
        contentType: 'application/json',
        dataType: 'json',
        data: data !== undefined ? JSON.stringify(data) : undefined,
        timeout: 15000,
      })
        .done((res) => resolve(res ? res.data : null))
        .fail((xhr, textStatus) => {
          if (textStatus === 'timeout') {
            reject(new ApiError(0, 'timeout', 'El servidor tardó demasiado en responder'));
            return;
          }
          const err = parseError(xhr);
          // token vencido o usuario eliminado: de vuelta al login
          if (err.status === 401 && token) {
            Session.clear();
            window.location.href = '/?expired=1';
          }
          reject(err);
        });
    });
  }

  return {
    get: (url) => request('GET', url),
    post: (url, data) => request('POST', url, data ?? {}),
    put: (url, data) => request('PUT', url, data),
    patch: (url, data) => request('PATCH', url, data),
    del: (url) => request('DELETE', url),
  };
})();

const UI = (() => {
  const TOAST_ICONS = {
    success: 'bi-check-circle-fill',
    danger: 'bi-exclamation-octagon-fill',
    warning: 'bi-exclamation-triangle-fill',
    info: 'bi-info-circle-fill',
  };

  function esc(str) {
    return $('<div>').text(str ?? '').html();
  }

  function toast(message, type = 'success') {
    let $wrap = $('#toast-area');
    if (!$wrap.length) {
      $wrap = $('<div id="toast-area" class="toast-container position-fixed bottom-0 end-0 p-3"></div>')
        .appendTo('body');
    }
    const $t = $(`
      <div class="toast align-items-center text-bg-${type} border-0" role="alert" aria-live="assertive">
        <div class="d-flex">
          <div class="toast-body"><i class="bi ${TOAST_ICONS[type] || TOAST_ICONS.info} me-2"></i>${esc(message)}</div>
          <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button>
        </div>
      </div>`).appendTo($wrap);
    const t = new bootstrap.Toast($t[0], { delay: type === 'danger' ? 6000 : 3500 });
    $t.on('hidden.bs.toast', () => $t.remove());
    t.show();
  }

  // Muestra un error de la API: campos inválidos en el form y el mensaje en un toast.
  function showError(err, $form) {
    if (!(err instanceof ApiError)) {
      console.error(err);
      toast('Algo salió mal en la página, recarga e intenta de nuevo', 'danger');
      return;
    }
    if ($form && Object.keys(err.fields).length) {
      fieldErrors($form, err.fields);
    }
    toast(err.message, err.status === 409 || err.status === 403 ? 'warning' : 'danger');
  }

  function clearFieldErrors($form) {
    $form.find('.is-invalid').removeClass('is-invalid');
    $form.find('.invalid-feedback.api').remove();
  }

  function fieldErrors($form, fields) {
    clearFieldErrors($form);
    Object.entries(fields).forEach(([name, msg]) => {
      const $input = $form.find(`[name="${name}"]`);
      if (!$input.length) return;
      $input.addClass('is-invalid');
      $(`<div class="invalid-feedback api">${esc(msg)}</div>`).insertAfter($input);
    });
  }

  function setLoading($btn, loading) {
    if (loading) {
      $btn.data('label', $btn.html()).prop('disabled', true)
        .html('<span class="spinner-border spinner-border-sm me-1"></span>Procesando…');
    } else {
      $btn.prop('disabled', false).html($btn.data('label'));
    }
  }

  function formData($form) {
    const out = {};
    $form.serializeArray().forEach(({ name, value }) => { out[name] = value; });
    return out;
  }

  function fmtDate(iso) {
    if (!iso) return '—';
    // las fechas límite se guardan como día (UTC), no se convierten de zona
    return iso.slice(0, 10).split('-').reverse().join('/');
  }

  function fmtDateTime(iso) {
    if (!iso) return '—';
    const d = new Date(iso);
    if (isNaN(d)) return '—';
    return d.toLocaleString('es-GT', { dateStyle: 'short', timeStyle: 'short' });
  }

  function initials(name) {
    return (name || '?').split(/\s+/).filter(Boolean).slice(0, 2).map((p) => p[0].toUpperCase()).join('');
  }

  // Modal de confirmación / texto reutilizable, devuelve una promesa.
  function prompt({ title, message, label, required = true, confirmText = 'Confirmar', variant = 'primary' }) {
    return new Promise((resolve) => {
      const withInput = !!label;
      const $m = $(`
        <div class="modal fade" tabindex="-1">
          <div class="modal-dialog modal-dialog-centered">
            <form class="modal-content" novalidate>
              <div class="modal-header">
                <h5 class="modal-title">${esc(title)}</h5>
                <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
              </div>
              <div class="modal-body">
                ${message ? `<p class="mb-2">${esc(message)}</p>` : ''}
                ${withInput ? `<label class="form-label">${esc(label)}</label>
                  <textarea class="form-control" name="value" rows="3" maxlength="500"></textarea>
                  <div class="invalid-feedback">Este campo es obligatorio</div>` : ''}
              </div>
              <div class="modal-footer">
                <button type="button" class="btn btn-outline-secondary" data-bs-dismiss="modal">Cancelar</button>
                <button type="submit" class="btn btn-${variant}">${esc(confirmText)}</button>
              </div>
            </form>
          </div>
        </div>`).appendTo('body');
      const modal = new bootstrap.Modal($m[0]);
      let result = null;

      $m.find('form').on('submit', (e) => {
        e.preventDefault();
        if (withInput) {
          const val = $m.find('[name=value]').val().trim();
          if (required && !val) {
            $m.find('[name=value]').addClass('is-invalid');
            return;
          }
          result = val;
        } else {
          result = true;
        }
        modal.hide();
      });
      // Bootstrap no soporta modales apilados de forma nativa: se sube el z-index
      // de este modal y su backdrop para que queden sobre el modal de la tarea.
      const stacked = $('.modal.show').length > 0;
      if (stacked) $m.css('z-index', 1065);
      $m.on('shown.bs.modal', () => {
        if (stacked) $('.modal-backdrop').last().css('z-index', 1060);
        $m.find('textarea').trigger('focus');
      });
      $m.on('hidden.bs.modal', () => {
        $m.remove();
        if ($('.modal.show').length) $('body').addClass('modal-open');
        resolve(result);
      });
      modal.show();
    });
  }

  const confirm = (title, message, variant = 'danger') =>
    prompt({ title, message, confirmText: 'Sí, continuar', variant });

  return { esc, toast, showError, fieldErrors, clearFieldErrors, setLoading, formData,
    fmtDate, fmtDateTime, initials, prompt, confirm };
})();

// Para páginas privadas: si no hay sesión, al login.
function requireSession() {
  if (!Session.token()) {
    window.location.replace('/');
    return false;
  }
  return true;
}

function renderNavbar(active) {
  const user = Session.user() || {};
  const isAdmin = user.role === 'admin';
  const link = (href, icon, text, key) =>
    `<li class="nav-item"><a class="nav-link ${active === key ? 'active' : ''}" href="${href}">
      <i class="bi ${icon} me-1"></i>${text}</a></li>`;

  $('#navbar').html(`
    <nav class="navbar navbar-expand-md navbar-dark bg-dark">
      <div class="container-fluid">
        <a class="navbar-brand fw-semibold" href="/dashboard"><i class="bi bi-kanban me-1"></i>TaskFlow</a>
        <button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#mainNav">
          <span class="navbar-toggler-icon"></span>
        </button>
        <div class="collapse navbar-collapse" id="mainNav">
          <ul class="navbar-nav me-auto">
            ${link('/dashboard', 'bi-speedometer2', 'Dashboard', 'dashboard')}
            ${isAdmin ? link('/users', 'bi-people', 'Usuarios', 'users') : ''}
          </ul>
          <div class="d-flex align-items-center gap-2 text-light small">
            <span class="avatar avatar-sm">${UI.esc(UI.initials(user.name))}</span>
            <span>${UI.esc(user.name || '')}</span>
            <span class="badge ${isAdmin ? 'text-bg-warning' : 'text-bg-secondary'}">${isAdmin ? 'Admin' : 'Miembro'}</span>
            <button id="btnLogout" class="btn btn-sm btn-outline-light ms-2" title="Cerrar sesión">
              <i class="bi bi-box-arrow-right"></i>
            </button>
          </div>
        </div>
      </div>
    </nav>`);

  $('#btnLogout').on('click', () => {
    Session.clear();
    window.location.href = '/';
  });

  // refresca rol/nombre por si el admin los cambió
  Api.get('/auth/me').then((me) => {
    const prev = Session.user() || {};
    Session.setUser(me);
    if (prev.role !== me.role) window.location.reload();
  }).catch(() => { /* el 401 ya redirige */ });
}
