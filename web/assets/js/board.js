// Tablero Kanban: render de columnas/tarjetas, drag & drop y alta de tareas.

const STATUS_ORDER = ['backlog', 'in_progress', 'review', 'done'];
const STATUS_LABELS = {
  backlog: 'Por hacer',
  in_progress: 'En progreso',
  review: 'En revisión / QA',
  done: 'Completado',
};
const PRIORITY_BADGE = {
  alta: '<span class="badge text-bg-danger">Alta</span>',
  media: '<span class="badge text-bg-warning">Media</span>',
  baja: '<span class="badge text-bg-secondary">Baja</span>',
};

function labelChips(labels) {
  if (!labels) return '';
  return labels.split(',').filter(Boolean)
    .map((l) => `<span class="badge rounded-pill label-chip">${UI.esc(l)}</span>`).join(' ');
}

const Board = {
  id: null,
  data: null,
  tasks: {}, // id -> card, para drag & drop

  async load({ silent = false } = {}) {
    try {
      const data = await Api.get(`/boards/${this.id}`);
      this.data = data;
      this.render();
    } catch (err) {
      if (err.status === 404 || err.status === 403) {
        $('#boardName').text('Tablero no disponible');
        $('#board').html(`<div class="alert alert-warning w-100">${UI.esc(err.message)}.
          <a href="/dashboard" class="alert-link">Volver al dashboard</a></div>`);
        return;
      }
      if (!silent) UI.showError(err);
    }
  },

  render() {
    const d = this.data;
    document.title = `${d.name} · TaskFlow`;
    $('#boardName').text(d.name);
    $('#crumbProject').text(d.project_name);
    this.tasks = {};

    $('#board').html(d.columns.map((col) => {
      col.cards.forEach((t) => { this.tasks[t.id] = t; });
      return `
        <section class="kanban-col" data-status="${col.status}">
          <header class="kanban-col-header">
            <span>${UI.esc(col.name)}</span>
            <span class="badge rounded-pill text-bg-light border">${col.cards.length}</span>
          </header>
          <div class="kanban-list" data-status="${col.status}">
            ${col.cards.map((t) => this.cardHtml(t)).join('') ||
              '<p class="text-center text-muted small my-3 empty-hint">Sin tareas</p>'}
          </div>
        </section>`;
    }).join(''));
  },

  cardHtml(t) {
    const p = t.progress || { total: 0, percent: 0 };
    const due = t.due_date ? `
      <span class="small ${t.overdue ? 'text-danger fw-semibold' : 'text-muted'}" title="Fecha límite">
        <i class="bi bi-calendar-event me-1"></i>${UI.fmtDate(t.due_date)}
      </span>` : '';
    const assignee = t.assignee
      ? `<span class="avatar avatar-sm" title="${UI.esc(t.assignee.name)}">${UI.esc(UI.initials(t.assignee.name))}</span>`
      : '<span class="text-muted small" title="Sin responsable"><i class="bi bi-person-dash"></i></span>';

    return `
      <article class="card task-card mb-2 ${t.overdue ? 'overdue' : ''}" draggable="true"
        data-id="${t.id}" data-priority="${t.priority}">
        <div class="card-body p-2">
          <div class="d-flex justify-content-between align-items-start gap-2 mb-1">
            <span class="fw-semibold small">${UI.esc(t.title)}</span>
            ${PRIORITY_BADGE[t.priority] || ''}
          </div>
          ${t.labels ? `<div class="mb-1">${labelChips(t.labels)}</div>` : ''}
          ${p.total ? `
            <div class="d-flex align-items-center gap-2 mb-1">
              <div class="progress flex-grow-1"><div class="progress-bar ${p.percent === 100 ? 'bg-success' : ''}"
                style="width:${p.percent}%"></div></div>
              <span class="small text-muted">${p.done}/${p.total}</span>
            </div>` : ''}
          <div class="d-flex justify-content-between align-items-center">
            <div>${due} ${t.overdue ? '<span class="badge text-bg-danger ms-1">Vencida</span>' : ''}</div>
            ${assignee}
          </div>
        </div>
      </article>`;
  },

  bindEvents() {
    const $board = $('#board');

    $board.on('click', '.task-card', (e) => {
      TaskModal.open($(e.currentTarget).data('id'));
    });

    // ---- drag & drop nativo ----
    $board.on('dragstart', '.task-card', (e) => {
      e.originalEvent.dataTransfer.setData('text/plain', $(e.currentTarget).data('id'));
      e.originalEvent.dataTransfer.effectAllowed = 'move';
      $(e.currentTarget).addClass('dragging');
    });
    $board.on('dragend', '.task-card', (e) => $(e.currentTarget).removeClass('dragging'));
    $board.on('dragover', '.kanban-list', (e) => {
      e.preventDefault();
      $(e.currentTarget).addClass('drag-over');
    });
    $board.on('dragleave', '.kanban-list', (e) => $(e.currentTarget).removeClass('drag-over'));
    $board.on('drop', '.kanban-list', (e) => {
      e.preventDefault();
      $(e.currentTarget).removeClass('drag-over');
      const id = parseInt(e.originalEvent.dataTransfer.getData('text/plain'), 10);
      const to = $(e.currentTarget).data('status');
      if (id) this.dropTask(id, to);
    });

    $('#btnRefresh').on('click', () => this.load());
  },

  // Decide qué endpoint usar según la dirección del movimiento. El backend
  // valida igual todo; aquí solo se piden los datos extra (nota/motivo).
  async dropTask(id, to) {
    const task = this.tasks[id];
    if (!task || task.status === to) return;

    const from = STATUS_ORDER.indexOf(task.status);
    const target = STATUS_ORDER.indexOf(to);
    try {
      if (target < from) {
        if (task.status === 'review' && to === 'in_progress') {
          const note = await UI.prompt({
            title: 'Devolver a En progreso', label: 'Defecto encontrado en la revisión',
            confirmText: 'Devolver', variant: 'warning',
          });
          if (!note) return;
          await Api.post(`/tasks/${id}/move`, { status: to, note });
        } else if (Session.isAdmin()) {
          const reason = await UI.prompt({
            title: `Retroceder a "${STATUS_LABELS[to]}"`,
            message: 'Acción de administrador: quedará registrada en la bitácora.',
            label: 'Motivo del retroceso', confirmText: 'Retroceder', variant: 'danger',
          });
          if (!reason) return;
          await Api.post(`/tasks/${id}/rollback`, { status: to, reason });
        } else {
          UI.toast('Solo un Administrador puede retroceder una tarea', 'warning');
          return;
        }
      } else {
        await Api.post(`/tasks/${id}/move`, { status: to });
      }
      UI.toast(`Tarea movida a ${STATUS_LABELS[to]}`);
    } catch (err) {
      UI.showError(err);
    }
    this.load({ silent: true });
  },

  bindNewTask() {
    const $modal = $('#newTaskModal');
    const $form = $('#newTaskForm');

    $modal.on('show.bs.modal', () => {
      $form[0].reset();
      UI.clearFieldErrors($form);
      const members = (this.data && this.data.members) || [];
      $form.find('[name=assignee_id]').html('<option value="">Sin asignar</option>' +
        members.map((m) => `<option value="${m.id}">${UI.esc(m.name)}</option>`).join(''));
    });
    $modal.on('shown.bs.modal', () => $form.find('[name=title]').trigger('focus'));

    $form.on('submit', async (e) => {
      e.preventDefault();
      const data = UI.formData($form);
      const errors = {};
      if (!data.title.trim()) errors.title = 'El título es obligatorio';
      if (data.start_date && data.due_date && data.due_date < data.start_date) {
        errors.due_date = 'La fecha límite no puede ser anterior a la de inicio';
      }
      if (Object.keys(errors).length) {
        UI.fieldErrors($form, errors);
        return;
      }
      data.assignee_id = data.assignee_id ? parseInt(data.assignee_id, 10) : null;

      const $btn = $form.find('[type=submit]');
      UI.setLoading($btn, true);
      try {
        await Api.post(`/boards/${this.id}/tasks`, data);
        bootstrap.Modal.getOrCreateInstance($modal[0]).hide();
        UI.toast('Tarea creada');
        this.load();
      } catch (err) {
        UI.showError(err, $form);
      } finally {
        UI.setLoading($btn, false);
      }
    });
  },
};

$(() => {
  if (!requireSession()) return;
  renderNavbar('board');

  const id = parseInt(new URLSearchParams(window.location.search).get('id'), 10);
  if (!id) {
    window.location.replace('/dashboard');
    return;
  }
  Board.id = id;
  Board.bindEvents();
  Board.bindNewTask();
  TaskModal.init();
  Board.load();
});
