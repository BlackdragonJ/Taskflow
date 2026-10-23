// Modal de detalle de tarea. Todo se edita por AJAX y al terminar se refresca el tablero.

const ACTIVITY_ICONS = {
  created: 'bi-plus-circle text-primary',
  moved: 'bi-arrow-right-circle text-info',
  rejected: 'bi-arrow-counterclockwise text-warning',
  rollback: 'bi-shield-exclamation text-danger',
  assigned: 'bi-person-check text-success',
  updated: 'bi-pencil text-secondary',
  subtask: 'bi-check2-square text-secondary',
};

const COMMENT_POLL_MS = 8000;

const TaskModal = {
  task: null,
  modal: null,
  pollTimer: null,
  commentIds: new Set(),
  lastCommentId: 0,

  init() {
    const el = document.getElementById('taskModal');
    this.modal = bootstrap.Modal.getOrCreateInstance(el);
    $(el).on('hidden.bs.modal', () => {
      this.stopPolling();
      this.task = null;
    });
    this.bindEvents();
  },

  async open(id) {
    $('#tmHeader').html('<div class="placeholder-glow w-50"><span class="placeholder col-12"></span></div>');
    $('#tmMain, #tmSide, #tmComments').empty();
    this.modal.show();
    try {
      this.task = await Api.get(`/tasks/${id}`);
      this.render();
      this.loadComments(true);
      this.startPolling();
    } catch (err) {
      this.modal.hide();
      UI.showError(err);
    }
  },

  async refresh() {
    if (!this.task) return;
    try {
      this.task = await Api.get(`/tasks/${this.task.id}`);
      this.render();
    } catch (err) {
      UI.showError(err);
    }
  },

  // aplica la respuesta de un endpoint que devuelve el detalle completo
  apply(detail) {
    this.task = detail;
    this.render();
    Board.load({ silent: true });
  },

  render() {
    this.renderHeader();
    this.renderMain();
    this.renderSide();
  },

  // Bloque editable con el lápiz a la derecha
  editable(field, html, { multiline = false, placeholder = '' } = {}) {
    const canEdit = this.task.can_edit;
    return `
      <div class="editable d-flex align-items-start gap-2" data-field="${field}" data-multiline="${multiline}">
        <div class="flex-grow-1 value">${html || `<span class="text-muted fst-italic">${placeholder}</span>`}</div>
        ${canEdit ? `<button type="button" class="btn-edit ms-auto" title="Editar">
          <i class="bi bi-pencil"></i></button>` : ''}
      </div>`;
  },

  renderHeader() {
    const t = this.task;
    $('#tmHeader').html(`
      <div class="flex-grow-1 me-3">
        <div class="small text-muted mb-1">
          <i class="bi bi-hash"></i>${t.id} ·
          <span class="badge text-bg-light border">${UI.esc(t.status_label)}</span>
          ${t.overdue ? '<span class="badge text-bg-danger ms-1"><i class="bi bi-alarm me-1"></i>Vencida</span>' : ''}
          ${t.status === 'done' ? '<span class="badge text-bg-success ms-1"><i class="bi bi-lock me-1"></i>Cerrada</span>' : ''}
        </div>
        <div class="h5 mb-0">${this.editable('title', UI.esc(t.title))}</div>
      </div>
      <button type="button" class="btn-close" data-bs-dismiss="modal"></button>`);
  },

  renderMain() {
    const t = this.task;
    const p = t.progress;
    const subs = (t.subtasks || []).map((s) => `
      <li class="list-group-item d-flex align-items-center gap-2 subtask-item ${s.done ? 'done' : ''}">
        <input class="form-check-input mt-0 sub-check" type="checkbox" data-id="${s.id}"
          ${s.done ? 'checked' : ''} ${t.can_edit ? '' : 'disabled'}>
        <span class="sub-title flex-grow-1">${UI.esc(s.title)}</span>
        <span class="badge ${s.done ? 'text-bg-success' : 'text-bg-light border'}">${s.done ? 'Completada' : 'Pendiente'}</span>
        ${t.can_edit ? `<button class="btn btn-sm btn-link text-danger p-0 btn-del btn-del-sub" data-id="${s.id}"
          title="Eliminar"><i class="bi bi-x-lg"></i></button>` : ''}
      </li>`).join('');

    $('#tmMain').html(`
      ${t.overdue ? `<div class="alert alert-danger py-2 small">
        <i class="bi bi-exclamation-triangle-fill me-1"></i>
        La fecha límite (${UI.fmtDate(t.due_date)}) ya pasó y la tarea no está completada.</div>` : ''}

      <div class="section-title">Descripción / criterio de aceptación</div>
      ${this.editable('description', UI.esc(t.description), { multiline: true, placeholder: 'Sin descripción' })}

      <div class="section-title mt-3">Etiquetas</div>
      ${this.editable('labels', labelChips(t.labels), { placeholder: 'Sin etiquetas' })}

      <div class="d-flex justify-content-between align-items-center mt-4">
        <div class="section-title mb-0"><i class="bi bi-check2-square me-1"></i>Subtareas</div>
        <span class="small text-muted">${p.done} de ${p.total} completadas</span>
      </div>
      <div class="progress my-2" role="progressbar" aria-valuenow="${p.percent}" aria-valuemin="0" aria-valuemax="100" style="height:18px">
        <div class="progress-bar ${p.percent === 100 ? 'bg-success' : ''}" style="width:${p.percent}%">${p.total ? p.percent + '%' : ''}</div>
      </div>
      <ul class="list-group mb-2">${subs || '<li class="list-group-item small text-muted">No hay subtareas</li>'}</ul>
      ${t.can_edit ? `
        <form id="subtaskForm" class="input-group input-group-sm" novalidate>
          <input type="text" class="form-control" name="title" maxlength="150" placeholder="Nueva subtarea…">
          <button class="btn btn-outline-primary" type="submit"><i class="bi bi-plus-lg"></i> Agregar</button>
        </form>` : ''}`);
  },

  renderSide() {
    const t = this.task;
    const dis = t.can_edit ? '' : 'disabled';

    // El estado actual nunca va en las opciones; el placeholder solo indica la acción.
    const moveOpts = (t.moves || []).map((m) => `
      <option value="${m.status}" data-kind="${m.kind}" ${m.blocked ? 'disabled' : ''}>
        ${UI.esc(m.label)}${m.blocked ? ' — bloqueado' : ''}
      </option>`).join('');
    const blocked = (t.moves || []).filter((m) => m.blocked)
      .map((m) => `<div class="form-text"><i class="bi bi-lock me-1"></i>${UI.esc(m.blocked)}</div>`).join('');

    const memberOpts = (t.members || []).map((m) =>
      `<option value="${m.id}" ${t.assignee_id === m.id ? 'selected' : ''}>${UI.esc(m.name)}</option>`).join('');
    // si el responsable ya no es miembro, que igual se vea
    const orphan = t.assignee && !(t.members || []).some((m) => m.id === t.assignee_id)
      ? `<option value="${t.assignee.id}" selected>${UI.esc(t.assignee.name)} (fuera del proyecto)</option>` : '';

    const rollbackItems = (t.rollback || []).map((r) =>
      `<option value="${r.status}">${UI.esc(r.label)}</option>`).join('');

    $('#tmSide').html(`
      <div class="card border-0 bg-light mb-3">
        <div class="card-body">
          <label class="section-title" for="statusSelect">Estado actual: ${UI.esc(t.status_label)}</label>
          ${(t.moves || []).length ? `
            <select id="statusSelect" class="form-select form-select-sm">
              <option value="" selected disabled hidden>Mover a…</option>
              ${moveOpts}
            </select>
            ${blocked}
            <div id="rejectBox" class="mt-2 d-none">
              <textarea class="form-control form-control-sm mb-2" rows="2" maxlength="500"
                placeholder="Describe el defecto encontrado"></textarea>
              <div class="d-flex gap-2 justify-content-end">
                <button class="btn btn-sm btn-outline-secondary" id="rejectCancel">Cancelar</button>
                <button class="btn btn-sm btn-warning" id="rejectConfirm">Devolver a En progreso</button>
              </div>
            </div>` :
            `<p class="small text-muted mb-0"><i class="bi bi-lock me-1"></i>
              ${t.status === 'done' ? 'Tarea completada y cerrada.' : 'No hay movimientos disponibles.'}</p>`}

          ${rollbackItems ? `
            <hr class="my-2">
            <button class="btn btn-sm btn-outline-danger w-100" type="button" data-bs-toggle="collapse" data-bs-target="#rollbackBox">
              <i class="bi bi-shield-exclamation me-1"></i>Retroceder estado (Admin)
            </button>
            <div class="collapse mt-2" id="rollbackBox">
              <select class="form-select form-select-sm mb-2" id="rollbackTo">${rollbackItems}</select>
              <textarea class="form-control form-control-sm mb-2" id="rollbackReason" rows="2" maxlength="500"
                placeholder="Motivo del retroceso (obligatorio)"></textarea>
              <button class="btn btn-sm btn-danger w-100" id="rollbackConfirm">Confirmar retroceso</button>
            </div>` : ''}
        </div>
      </div>

      <div class="mb-3">
        <label class="section-title" for="prioritySelect">Prioridad</label>
        <select id="prioritySelect" class="form-select form-select-sm" ${dis}>
          <option value="alta" ${t.priority === 'alta' ? 'selected' : ''}>Alta</option>
          <option value="media" ${t.priority === 'media' ? 'selected' : ''}>Media</option>
          <option value="baja" ${t.priority === 'baja' ? 'selected' : ''}>Baja</option>
        </select>
      </div>

      <div class="mb-3">
        <label class="section-title" for="assigneeSelect">Responsable</label>
        <select id="assigneeSelect" class="form-select form-select-sm" ${dis}>
          <option value="0" ${t.assignee_id ? '' : 'selected'}>Sin asignar</option>
          ${orphan}${memberOpts}
        </select>
      </div>

      <div class="row g-2 mb-3">
        <div class="col-6">
          <label class="section-title" for="startDate">Inicio</label>
          <input type="date" id="startDate" class="form-control form-control-sm date-input" data-field="start_date"
            value="${t.start_date ? t.start_date.slice(0, 10) : ''}" ${dis}>
        </div>
        <div class="col-6">
          <label class="section-title" for="dueDate">Fecha límite</label>
          <input type="date" id="dueDate" class="form-control form-control-sm date-input ${t.overdue ? 'is-invalid' : ''}"
            data-field="due_date" value="${t.due_date ? t.due_date.slice(0, 10) : ''}" ${dis}>
        </div>
      </div>

      <dl class="row small mb-3">
        <dt class="col-5 text-muted fw-normal">Creada</dt><dd class="col-7">${UI.fmtDateTime(t.created_at)}</dd>
        <dt class="col-5 text-muted fw-normal">Creada por</dt><dd class="col-7">${UI.esc(t.created_by ? t.created_by.name : '—')}</dd>
        ${t.closed_at ? `<dt class="col-5 text-muted fw-normal">Cerrada</dt><dd class="col-7 text-success">${UI.fmtDateTime(t.closed_at)}</dd>` : ''}
      </dl>

      <div class="section-title"><i class="bi bi-clock-history me-1"></i>Bitácora</div>
      <ul class="list-unstyled activity-list d-flex flex-column gap-2">
        ${(t.activity || []).map((a) => `
          <li>
            <i class="bi ${ACTIVITY_ICONS[a.action] || 'bi-dot'} me-1"></i>
            <strong>${UI.esc(a.user ? a.user.name : '¿?')}</strong> ${UI.esc(a.detail)}
            <div class="text-muted">${UI.fmtDateTime(a.created_at)}</div>
          </li>`).join('') || '<li class="text-muted">Sin actividad</li>'}
      </ul>

      ${t.can_delete ? `<button class="btn btn-sm btn-outline-danger w-100 mt-2" id="btnDeleteTask">
        <i class="bi bi-trash me-1"></i>Eliminar tarea</button>` : ''}`);
  },

  async patch(data, $revert) {
    try {
      const detail = await Api.patch(`/tasks/${this.task.id}`, data);
      this.apply(detail);
      UI.toast('Cambios guardados');
    } catch (err) {
      UI.showError(err);
      if ($revert) this.renderSide(); // vuelve el select/fecha a su valor real
    }
  },

  async move(status, note) {
    try {
      const detail = await Api.post(`/tasks/${this.task.id}/move`, { status, note });
      this.apply(detail);
      UI.toast(`Tarea movida a ${detail.status_label}`);
    } catch (err) {
      UI.showError(err);
      this.renderSide();
    }
  },

  // ---- comentarios ----

  commentHtml(c) {
    const name = c.user ? c.user.name : '¿?';
    return `
      <div class="comment d-flex gap-2" data-id="${c.id}">
        <span class="avatar avatar-sm">${UI.esc(UI.initials(name))}</span>
        <div class="flex-grow-1">
          <div class="small"><strong>${UI.esc(name)}</strong>
            <span class="text-muted ms-1">${UI.fmtDateTime(c.created_at)}</span></div>
          <div class="body small">${UI.esc(c.body)}</div>
        </div>
      </div>`;
  },

  appendComments(list) {
    const $box = $('#tmComments');
    const nearBottom = $box[0].scrollHeight - $box.scrollTop() - $box.outerHeight() < 40;
    list.forEach((c) => {
      if (this.commentIds.has(c.id)) return;
      this.commentIds.add(c.id);
      this.lastCommentId = Math.max(this.lastCommentId, c.id);
      $box.find('.no-comments').remove();
      $box.append(this.commentHtml(c));
    });
    if (nearBottom || list.length) $box.scrollTop($box[0].scrollHeight);
  },

  async loadComments(reset = false) {
    if (!this.task) return;
    const taskId = this.task.id;
    if (reset) {
      this.commentIds.clear();
      this.lastCommentId = 0;
      $('#tmComments').html('<p class="small text-muted mb-0 no-comments">Sin comentarios todavía</p>');
    }
    try {
      const list = await Api.get(`/tasks/${taskId}/comments${this.lastCommentId ? `?after=${this.lastCommentId}` : ''}`);
      // el usuario pudo cerrar/cambiar de tarea mientras llegaba la respuesta
      if (!this.task || this.task.id !== taskId) return;
      this.appendComments(list || []);
    } catch (err) {
      // en el polling no se molesta con toasts repetidos
      if (reset) UI.showError(err);
      else console.warn('polling comentarios:', err.message);
    }
  },

  startPolling() {
    this.stopPolling();
    this.pollTimer = setInterval(() => this.loadComments(), COMMENT_POLL_MS);
  },

  stopPolling() {
    if (this.pollTimer) clearInterval(this.pollTimer);
    this.pollTimer = null;
  },

  // ---- eventos (delegados, se enlazan una sola vez) ----

  bindEvents() {
    const $modal = $('#taskModal');

    // lápiz → editor inline
    $modal.on('click', '.editable .btn-edit', (e) => {
      const $ed = $(e.currentTarget).closest('.editable');
      const field = $ed.data('field');
      const multiline = $ed.data('multiline') === true;
      const current = this.task[field] || '';
      const input = multiline
        ? `<textarea class="form-control form-control-sm" rows="4">${UI.esc(current)}</textarea>`
        : `<input type="text" class="form-control form-control-sm" value="${UI.esc(current)}">`;

      $ed.removeClass('editable').addClass('editing').html(`
        <div class="flex-grow-1">
          ${input}
          ${field === 'labels' ? '<div class="form-text">Separadas por coma</div>' : ''}
          <div class="mt-2 d-flex gap-2">
            <button type="button" class="btn btn-sm btn-primary btn-save"><i class="bi bi-check-lg"></i> Guardar</button>
            <button type="button" class="btn btn-sm btn-outline-secondary btn-cancel">Cancelar</button>
          </div>
        </div>`);
      // directo en el input: así el Escape no llega al modal y no lo cierra
      $ed.find('input, textarea').on('keydown', (ev) => {
        if (ev.key === 'Escape') {
          ev.stopPropagation();
          this.render();
        } else if (ev.key === 'Enter' && (ev.target.tagName === 'INPUT' || ev.ctrlKey)) {
          ev.preventDefault();
          $ed.find('.btn-save').trigger('click');
        }
      }).trigger('focus');
    });

    $modal.on('click', '.editing .btn-cancel', () => this.render());
    $modal.on('click', '.editing .btn-save', (e) => {
      const $ed = $(e.currentTarget).closest('.editing');
      const field = $ed.data('field');
      const value = $ed.find('input, textarea').val();
      if (field === 'title' && !value.trim()) {
        $ed.find('input').addClass('is-invalid');
        UI.toast('El título es obligatorio', 'warning');
        return;
      }
      this.patch({ [field]: value });
    });

    // selects directos
    $modal.on('change', '#prioritySelect', (e) => this.patch({ priority: e.target.value }, true));
    $modal.on('change', '#assigneeSelect', (e) => this.patch({ assignee_id: parseInt(e.target.value, 10) }, true));
    $modal.on('change', '.date-input', (e) => {
      const start = $('#startDate').val();
      const due = $('#dueDate').val();
      if (start && due && due < start) {
        UI.toast('La fecha límite no puede ser anterior a la de inicio', 'warning');
        this.renderSide();
        return;
      }
      this.patch({ [$(e.target).data('field')]: e.target.value }, true);
    });

    // cambio de estado
    $modal.on('change', '#statusSelect', (e) => {
      const $opt = $(e.target).find(':selected');
      if ($opt.data('kind') === 'reject') {
        $('#rejectBox').removeClass('d-none').find('textarea').trigger('focus');
        return;
      }
      this.move(e.target.value);
    });
    $modal.on('click', '#rejectCancel', () => this.renderSide());
    $modal.on('click', '#rejectConfirm', () => {
      const note = $('#rejectBox textarea').val().trim();
      if (!note) {
        $('#rejectBox textarea').addClass('is-invalid');
        return;
      }
      this.move('in_progress', note);
    });

    // retroceso (admin)
    $modal.on('click', '#rollbackConfirm', async () => {
      const status = $('#rollbackTo').val();
      const reason = $('#rollbackReason').val().trim();
      if (!reason) {
        $('#rollbackReason').addClass('is-invalid');
        return;
      }
      try {
        const detail = await Api.post(`/tasks/${this.task.id}/rollback`, { status, reason });
        this.apply(detail);
        UI.toast(`Tarea retrocedida a ${detail.status_label}`, 'warning');
      } catch (err) {
        UI.showError(err);
      }
    });

    // subtareas
    $modal.on('submit', '#subtaskForm', async (e) => {
      e.preventDefault();
      const $input = $(e.target).find('[name=title]');
      const title = $input.val().trim();
      if (!title) {
        $input.addClass('is-invalid');
        return;
      }
      try {
        await Api.post(`/tasks/${this.task.id}/subtasks`, { title });
        await this.refresh();
        Board.load({ silent: true });
        $('#subtaskForm [name=title]').trigger('focus');
      } catch (err) {
        UI.showError(err);
      }
    });
    $modal.on('change', '.sub-check', async (e) => {
      const id = $(e.target).data('id');
      try {
        await Api.patch(`/tasks/${this.task.id}/subtasks/${id}`, { done: e.target.checked });
        await this.refresh();
        Board.load({ silent: true });
      } catch (err) {
        e.target.checked = !e.target.checked;
        UI.showError(err);
      }
    });
    $modal.on('click', '.btn-del-sub', async (e) => {
      const id = $(e.currentTarget).data('id');
      try {
        await Api.del(`/tasks/${this.task.id}/subtasks/${id}`);
        await this.refresh();
        Board.load({ silent: true });
      } catch (err) {
        UI.showError(err);
      }
    });

    // comentarios
    $('#commentForm').on('submit', async (e) => {
      e.preventDefault();
      const $txt = $('#commentForm [name=body]');
      const body = $txt.val().trim();
      if (!body) {
        $txt.addClass('is-invalid');
        return;
      }
      $txt.removeClass('is-invalid');
      const $btn = $('#commentForm [type=submit]');
      UI.setLoading($btn, true);
      try {
        const c = await Api.post(`/tasks/${this.task.id}/comments`, { body });
        this.appendComments([c]);
        $txt.val('');
      } catch (err) {
        UI.showError(err);
      } finally {
        UI.setLoading($btn, false);
      }
    });
    $('#commentForm [name=body]').on('keydown', (e) => {
      if (e.key === 'Enter' && e.ctrlKey) $('#commentForm').trigger('submit');
    });

    $modal.on('click', '#btnDeleteTask', async () => {
      const ok = await UI.confirm('Eliminar tarea', `¿Eliminar "${this.task.title}"? También se borran sus subtareas, comentarios y bitácora.`);
      if (!ok) return;
      try {
        await Api.del(`/tasks/${this.task.id}`);
        this.modal.hide();
        UI.toast('Tarea eliminada');
        Board.load({ silent: true });
      } catch (err) {
        UI.showError(err);
      }
    });
  },
};
