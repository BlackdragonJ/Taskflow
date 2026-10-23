// Dashboard: métricas por proyecto + administración de proyectos/tableros/miembros.

const Dashboard = {
  projects: [],
  stats: {},
  isAdmin: Session.isAdmin(),
  currentProjectId: null,

  async load() {
    try {
      const [projects, stats] = await Promise.all([Api.get('/projects'), Api.get('/dashboard')]);
      this.projects = projects || [];
      this.stats = {};
      (stats || []).forEach((s) => { this.stats[s.project_id] = s; });
      this.renderTotals(stats || []);
      this.renderProjects();
    } catch (err) {
      UI.showError(err);
      $('#projects').html(`<div class="col-12"><div class="alert alert-danger">
        No se pudo cargar el panel. <a href="#" class="alert-link" id="retry">Reintentar</a></div></div>`);
      $('#retry').on('click', (e) => { e.preventDefault(); this.load(); });
    }
  },

  renderTotals(stats) {
    const sum = (k) => stats.reduce((acc, s) => acc + (s[k] || 0), 0);
    const cards = [
      { label: 'Total de tareas', value: sum('total'), icon: 'bi-list-task', cls: 'text-primary' },
      { label: 'Completadas', value: sum('done'), icon: 'bi-check2-circle', cls: 'text-success' },
      { label: 'En progreso', value: sum('in_progress'), icon: 'bi-arrow-repeat', cls: 'text-info' },
      { label: 'Vencidas', value: sum('overdue'), icon: 'bi-alarm', cls: 'text-danger' },
    ];
    $('#totals').html(cards.map((c) => `
      <div class="col-6 col-lg-3">
        <div class="card stat-card border-0 shadow-sm h-100">
          <div class="card-body d-flex justify-content-between align-items-center">
            <div>
              <div class="text-muted small">${c.label}</div>
              <div class="stat-value ${c.cls}">${c.value}</div>
            </div>
            <i class="bi ${c.icon} stat-icon ${c.cls}"></i>
          </div>
        </div>
      </div>`).join(''));
  },

  renderProjects() {
    const $wrap = $('#projects');
    if (!this.projects.length) {
      $wrap.html(`<div class="col-12"><div class="alert alert-light border text-center py-4 mb-0">
        <i class="bi bi-folder2-open fs-3 d-block mb-2 text-muted"></i>
        ${this.isAdmin ? 'Aún no hay proyectos. Crea el primero con <strong>Nuevo proyecto</strong>.'
          : 'Todavía no te han asignado a ningún proyecto. Pídele acceso a un administrador.'}
      </div></div>`);
      return;
    }

    $wrap.html(this.projects.map((p) => {
      const s = this.stats[p.id] || { total: 0, done: 0, in_progress: 0, review: 0, backlog: 0, overdue: 0 };
      const pct = s.total ? Math.round((s.done * 100) / s.total) : 0;
      const boards = (p.boards || []).map((b) => `
        <div class="d-flex align-items-center justify-content-between border rounded px-2 py-1 mb-1">
          <a href="/board?id=${b.id}" class="text-decoration-none flex-grow-1">
            <i class="bi bi-kanban me-1"></i>${UI.esc(b.name)}</a>
          ${this.isAdmin ? `<button class="btn btn-sm btn-link text-danger p-0 btn-del-board" data-id="${b.id}"
            data-name="${UI.esc(b.name)}" title="Eliminar tablero"><i class="bi bi-trash"></i></button>` : ''}
        </div>`).join('');

      return `
      <div class="col-md-6 col-xl-4">
        <div class="card project-card shadow-sm border-0 h-100">
          <div class="card-body d-flex flex-column">
            <div class="d-flex justify-content-between align-items-start mb-1">
              <h3 class="h6 mb-0">${UI.esc(p.name)}</h3>
              ${this.isAdmin ? `
              <div class="dropdown">
                <button class="btn btn-sm btn-light" data-bs-toggle="dropdown"><i class="bi bi-three-dots"></i></button>
                <ul class="dropdown-menu dropdown-menu-end">
                  <li><a class="dropdown-item btn-edit-project" href="#" data-id="${p.id}"><i class="bi bi-pencil me-2"></i>Editar</a></li>
                  <li><a class="dropdown-item btn-members" href="#" data-id="${p.id}"><i class="bi bi-people me-2"></i>Miembros</a></li>
                  <li><a class="dropdown-item btn-new-board" href="#" data-id="${p.id}"><i class="bi bi-plus-square me-2"></i>Nuevo tablero</a></li>
                  <li><hr class="dropdown-divider"></li>
                  <li><a class="dropdown-item text-danger btn-del-project" href="#" data-id="${p.id}"><i class="bi bi-trash me-2"></i>Eliminar</a></li>
                </ul>
              </div>` : ''}
            </div>
            <p class="text-muted small mb-3">${UI.esc(p.description) || '<em>Sin descripción</em>'}</p>

            <div class="row g-2 text-center mb-3">
              ${this.miniStat('Total', s.total, 'primary')}
              ${this.miniStat('Completadas', s.done, 'success')}
              ${this.miniStat('En progreso', s.in_progress, 'info')}
              ${this.miniStat('Vencidas', s.overdue, 'danger')}
            </div>
            <div class="d-flex justify-content-between small text-muted mb-1">
              <span>Por hacer: ${s.backlog} · En revisión: ${s.review}</span><span>${pct}%</span>
            </div>
            <div class="progress mb-3" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100">
              <div class="progress-bar bg-success" style="width:${pct}%"></div>
            </div>

            <div class="mt-auto">
              <div class="section-title">Tableros</div>
              ${boards || '<p class="small text-muted mb-0">Sin tableros</p>'}
            </div>
          </div>
        </div>
      </div>`;
    }).join(''));
  },

  miniStat(label, value, color) {
    return `<div class="col-3">
      <div class="rounded bg-${color}-subtle py-2">
        <div class="fw-bold text-${color}-emphasis">${value}</div>
        <div class="text-muted" style="font-size:.68rem">${label}</div>
      </div></div>`;
  },

  // ---- admin ----

  bindAdmin() {
    if (!this.isAdmin) return;
    $('#btnNewProject').removeClass('d-none');

    const $modal = $('#projectModal');
    const $form = $('#projectForm');

    $modal.on('show.bs.modal', (e) => {
      // abierto desde el botón "Nuevo proyecto" → limpiar
      if (e.relatedTarget) {
        $form[0].reset();
        $form.find('[name=id]').val('');
        $modal.find('.modal-title').text('Nuevo proyecto');
      }
      UI.clearFieldErrors($form);
    });

    $form.on('submit', async (e) => {
      e.preventDefault();
      const data = UI.formData($form);
      if (!data.name.trim()) {
        UI.fieldErrors($form, { name: 'El nombre es obligatorio' });
        return;
      }
      const $btn = $form.find('[type=submit]');
      UI.setLoading($btn, true);
      try {
        if (data.id) {
          await Api.put(`/projects/${data.id}`, { name: data.name, description: data.description });
          UI.toast('Proyecto actualizado');
        } else {
          await Api.post('/projects', { name: data.name, description: data.description });
          UI.toast('Proyecto creado');
        }
        bootstrap.Modal.getOrCreateInstance($modal[0]).hide();
        this.load();
      } catch (err) {
        UI.showError(err, $form);
      } finally {
        UI.setLoading($btn, false);
      }
    });

    $('#projects')
      .on('click', '.btn-edit-project', (e) => {
        e.preventDefault();
        const p = this.projects.find((x) => x.id === $(e.currentTarget).data('id'));
        if (!p) return;
        $form.find('[name=id]').val(p.id);
        $form.find('[name=name]').val(p.name);
        $form.find('[name=description]').val(p.description);
        $modal.find('.modal-title').text('Editar proyecto');
        bootstrap.Modal.getOrCreateInstance($modal[0]).show();
      })
      .on('click', '.btn-del-project', async (e) => {
        e.preventDefault();
        const id = $(e.currentTarget).data('id');
        const ok = await UI.confirm('Eliminar proyecto',
          'Se eliminarán todos sus tableros, tareas y comentarios. Esta acción no se puede deshacer.');
        if (!ok) return;
        try {
          await Api.del(`/projects/${id}`);
          UI.toast('Proyecto eliminado');
          this.load();
        } catch (err) { UI.showError(err); }
      })
      .on('click', '.btn-new-board', async (e) => {
        e.preventDefault();
        const id = $(e.currentTarget).data('id');
        const name = await UI.prompt({ title: 'Nuevo tablero', label: 'Nombre del tablero', confirmText: 'Crear' });
        if (!name) return;
        try {
          await Api.post(`/projects/${id}/boards`, { name });
          UI.toast('Tablero creado');
          this.load();
        } catch (err) { UI.showError(err); }
      })
      .on('click', '.btn-del-board', async (e) => {
        e.preventDefault();
        const $b = $(e.currentTarget);
        const ok = await UI.confirm('Eliminar tablero', `¿Eliminar "${$b.data('name')}" y todas sus tareas?`);
        if (!ok) return;
        try {
          await Api.del(`/boards/${$b.data('id')}`);
          UI.toast('Tablero eliminado');
          this.load();
        } catch (err) { UI.showError(err); }
      })
      .on('click', '.btn-members', (e) => {
        e.preventDefault();
        this.openMembers($(e.currentTarget).data('id'));
      });

    $('#addMemberForm').on('submit', async (e) => {
      e.preventDefault();
      const userId = parseInt($('#addMemberForm [name=user_id]').val(), 10);
      if (!userId) {
        UI.toast('Selecciona un usuario', 'warning');
        return;
      }
      try {
        await Api.post(`/projects/${this.currentProjectId}/members`, { user_id: userId });
        UI.toast('Miembro agregado');
        this.openMembers(this.currentProjectId);
      } catch (err) { UI.showError(err); }
    });

    $('#membersList').on('click', '.btn-remove-member', async (e) => {
      const uid = $(e.currentTarget).data('id');
      try {
        await Api.del(`/projects/${this.currentProjectId}/members/${uid}`);
        UI.toast('Miembro removido');
        this.openMembers(this.currentProjectId);
      } catch (err) { UI.showError(err); }
    });
  },

  async openMembers(projectId) {
    this.currentProjectId = projectId;
    const $modal = $('#membersModal');
    try {
      const [project, users] = await Promise.all([Api.get(`/projects/${projectId}`), Api.get('/users')]);
      const memberIds = new Set((project.members || []).map((m) => m.id));
      $modal.find('.project-name').text(project.name);

      const candidates = users.filter((u) => u.active && !memberIds.has(u.id));
      $('#addMemberForm [name=user_id]').html(
        `<option value="">${candidates.length ? 'Selecciona un usuario…' : 'No hay más usuarios disponibles'}</option>` +
        candidates.map((u) => `<option value="${u.id}">${UI.esc(u.name)} (${UI.esc(u.email)})</option>`).join(''));

      $('#membersList').html((project.members || []).map((m) => `
        <li class="list-group-item d-flex align-items-center gap-2">
          <span class="avatar avatar-sm">${UI.esc(UI.initials(m.name))}</span>
          <div class="flex-grow-1">
            <div>${UI.esc(m.name)} ${m.id === project.owner_id ? '<span class="badge text-bg-light border">creador</span>' : ''}</div>
            <div class="small text-muted">${UI.esc(m.email)}</div>
          </div>
          ${m.id !== project.owner_id ? `<button class="btn btn-sm btn-outline-danger btn-remove-member" data-id="${m.id}" title="Quitar">
            <i class="bi bi-person-dash"></i></button>` : ''}
        </li>`).join(''));

      bootstrap.Modal.getOrCreateInstance($modal[0]).show();
    } catch (err) {
      UI.showError(err);
    }
  },
};

$(() => {
  if (!requireSession()) return;
  renderNavbar('dashboard');
  Dashboard.bindAdmin();
  Dashboard.load();
});
