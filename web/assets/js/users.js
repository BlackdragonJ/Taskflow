// Administración de usuarios (solo admin).

const Users = {
  list: [],

  async load() {
    try {
      this.list = await Api.get('/users');
      this.render();
    } catch (err) {
      UI.showError(err);
      $('#usersBody').html(`<tr><td colspan="4" class="text-center text-danger py-4">${UI.esc(err.message)}</td></tr>`);
    }
  },

  render() {
    const me = Session.user() || {};
    $('#usersBody').html(this.list.map((u) => {
      const self = u.id === me.id;
      return `
        <tr data-id="${u.id}" class="${u.active ? '' : 'table-secondary'}">
          <td>
            <div class="d-flex align-items-center gap-2">
              <span class="avatar">${UI.esc(UI.initials(u.name))}</span>
              <div>
                <div class="fw-semibold">${UI.esc(u.name)} ${self ? '<span class="badge text-bg-light border">tú</span>' : ''}</div>
                <div class="small text-muted">${UI.esc(u.email)}</div>
              </div>
            </div>
          </td>
          <td>
            <select class="form-select form-select-sm role-select" ${self ? 'disabled title="No puedes cambiar tu propio rol"' : ''}>
              <option value="member" ${u.role === 'member' ? 'selected' : ''}>Miembro</option>
              <option value="admin" ${u.role === 'admin' ? 'selected' : ''}>Administrador</option>
            </select>
          </td>
          <td class="text-center">
            <div class="form-check form-switch d-inline-block">
              <input class="form-check-input active-switch" type="checkbox" role="switch"
                ${u.active ? 'checked' : ''} ${self ? 'disabled' : ''}>
            </div>
          </td>
          <td class="small text-muted">${UI.fmtDateTime(u.created_at)}</td>
        </tr>`;
    }).join(''));
  },

  async update(id, data) {
    try {
      const updated = await Api.patch(`/users/${id}`, data);
      const idx = this.list.findIndex((u) => u.id === id);
      if (idx >= 0) this.list[idx] = updated;
      UI.toast('Usuario actualizado');
    } catch (err) {
      UI.showError(err);
    }
    this.render();
  },

  bind() {
    $('#usersBody')
      .on('change', '.role-select', (e) => {
        const id = $(e.target).closest('tr').data('id');
        this.update(id, { role: e.target.value });
      })
      .on('change', '.active-switch', (e) => {
        const id = $(e.target).closest('tr').data('id');
        this.update(id, { active: e.target.checked });
      });
  },
};

$(() => {
  if (!requireSession()) return;
  if (!Session.isAdmin()) {
    window.location.replace('/dashboard');
    return;
  }
  renderNavbar('users');
  Users.bind();
  Users.load();
});
