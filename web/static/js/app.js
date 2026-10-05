(() => {
  'use strict';
  const $ = selector => document.querySelector(selector);
  const eventSource = event => {
    const source = event.detail?.ctx?.sourceElement ?? event.target;
    return source instanceof Element ? source : null;
  };
  let root, socket, connecting = false, retry = 1000, reconnectTimer, refreshTimer;
  let controller = new AbortController(), refreshing = false, pending = false, reconcile = false;
  let lastRead = '', readBusy = false, sidebarBusy = false, toastTimer;
  let observedComposer;
  const composerObserver = new ResizeObserver(syncFooterHeight);
  const drafts = new Map(); // In memory only: navigation keeps a draft, reload clears it.
  const nearBottom = () => {
    const feed = $('#message-feed');
    return feed && feed.scrollHeight - feed.scrollTop - feed.clientHeight < 100;
  };
  function toast(message) {
    const node = $('#app-toast');
    if (!node) return;
    node.textContent = message;
    node.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { node.hidden = true; }, 6000);
  }
  function connection(connected) {
    document.querySelectorAll('[data-connection]').forEach(node => {
      node.classList.toggle('connected', connected);
      node.setAttribute('aria-label', connected ? 'Соединение установлено' : 'Восстанавливаем соединение');
      node.title = node.getAttribute('aria-label');
    });
  }
  async function request(path, options = {}) {
    const response = await fetch(path, { credentials: 'same-origin', cache: 'no-store', signal: controller.signal, ...options });
    const redirect = response.headers.get('HX-Redirect');
    if (redirect || response.redirected) {
      window.location.assign(redirect || response.url);
      throw new Error('redirect');
    }
    if (!response.ok) {
      const error = new Error('request failed');
      error.status = response.status;
      throw error;
    }
    return response;
  }
  function resize(textarea) {
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${Math.min(textarea.scrollHeight, 132)}px`;
  }
  function syncFooterHeight() {
    if (!root || !observedComposer?.isConnected) return;
    const height = `${observedComposer.getBoundingClientRect().height}px`;
    if (root.style.getPropertyValue('--app-composer-height') !== height) {
      root.style.setProperty('--app-composer-height', height);
    }
  }
  function observeComposer() {
    const next = $('#composer');
    if (observedComposer !== next) {
      composerObserver.disconnect();
      observedComposer = next;
      root?.style.removeProperty('--app-composer-height');
      if (next) composerObserver.observe(next);
    }
    syncFooterHeight();
  }
  function scrollBottom() {
    const feed = $('#message-feed');
    if (feed) feed.scrollTop = feed.scrollHeight;
    const button = $('[data-new-messages]');
    if (button) button.hidden = true;
    markRead();
  }
  async function markRead() {
    const chat = root?.dataset.activeChat;
    const id = Array.from(document.querySelectorAll('#message-feed [data-message-id]')).at(-1)?.dataset.messageId;
    if (!chat || !id || document.hidden || !nearBottom() || readBusy || lastRead === `${chat}/${id}`) return;
    readBusy = true;
    try {
      await request(`/app/chats/${chat}/read`, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'HX-Request': 'true' }, body: new URLSearchParams({ message_id: id }) });
      lastRead = `${chat}/${id}`;
      if (root?.dataset.activeChat === chat) refreshSidebar();
    } catch (_) { /* A later scroll, visibility change or reconnect retries the acknowledgement. */ }
    finally { readBusy = false; }
  }
  function filterChats() {
    const query = $('[data-chat-search]')?.value.toLocaleLowerCase().trim() || '';
    document.querySelectorAll('[data-chat-search-value]').forEach(row => {
      row.hidden = !row.dataset.chatSearchValue.toLocaleLowerCase().includes(query);
    });
  }
  async function refreshSidebar() {
    if (!root || sidebarBusy) return;
    sidebarBusy = true;
    const current = root, list = $('#chat-list'), scroll = list?.scrollTop;
    try {
      const response = await request(`/app/chats/list?active=${encodeURIComponent(root.dataset.activeChat || '')}`, { headers: { 'HX-Request': 'true' } });
      const html = await response.text();
      if (root !== current || !list?.isConnected) return;
      await htmx.swap({ text: html, target: list, swap: 'innerHTML' });
      list.scrollTop = scroll;
      filterChats();
    } catch (_) { /* Keep the current list when offline. */ }
    finally { sidebarBusy = false; }
  }
  function scheduleRefresh(full = false) {
    pending = true;
    reconcile ||= full;
    clearTimeout(refreshTimer);
    refreshTimer = setTimeout(refreshMessages, 80);
  }
  async function refreshMessages() {
    if (refreshing || !root?.dataset.activeChat || !$('#message-feed')) return;
    if ($('.message-edit')) return; // Keep an in-progress edit; retry after save/cancel.
    refreshing = true;
    const current = root, chat = current.dataset.activeChat;
    try {
      let batches = 0;
      do {
        pending = false;
        const full = reconcile || !$('#message-watermark')?.dataset.cursor;
        reconcile = false;
        const cursor = $('#message-watermark')?.dataset.cursor;
        const path = `/app/chats/${chat}/messages` + (full ? '' : `?after=1&cursor=${encodeURIComponent(cursor)}`);
        const response = await request(path, { headers: { 'HX-Request': 'true' } });
        const html = await response.text();
        if (root !== current) return;
        const fragment = document.createElement('template');
        fragment.innerHTML = html; // Only escaped HTML from our own templ handlers.
        const rows = fragment.content.querySelectorAll('[data-message-id]');
        const count = rows.length, stick = nearBottom();
        if (!full) rows.forEach(row => {
          if (document.getElementById(row.id)) row.remove();
          else row.classList.add('message-arrived');
        });
        await htmx.swap({ text: fragment.innerHTML, target: full ? '#message-feed' : '#message-list', swap: full ? 'innerHTML' : 'beforeend' });
        if (root !== current) return;
        if ($('#message-list')?.children.length) $('[data-feed-empty]')?.remove();
        if (stick) scrollBottom();
        else if (count && $('[data-new-messages]')) $('[data-new-messages]').hidden = false;
        pending ||= !full && count === 50;
      } while (pending && ++batches < 10);
    } catch (error) {
      if ([401, 403, 404].includes(error.status)) {
        pending = false;
        if (root === current) toast('Этот разговор больше недоступен.');
      } else if (error.name !== 'AbortError') pending = true;
    } finally {
      refreshing = false;
      if (root === current) refreshSidebar();
      if (pending && root?.dataset.activeChat) {
        clearTimeout(refreshTimer);
        refreshTimer = setTimeout(refreshMessages, 2000);
      }
    }
  }
  async function connect() {
    if (!root || socket || connecting) return;
    connecting = true;
    try {
      const response = await request('/app/session', { method: 'POST' });
      const { access_token } = await response.json();
      if (!root) return;
      const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/api/v1/ws`);
      socket = ws;
      ws.addEventListener('open', () => ws.send(JSON.stringify({ type: 'authenticate', access_token })));
      ws.addEventListener('message', event => {
        let message;
        try { message = JSON.parse(event.data); } catch (_) { return; }
        if (message.type === 'authenticated') {
          retry = 1000; connection(true); scheduleRefresh(true); refreshSidebar(); return;
        }
        const data = message.data;
        if (!data) return;
        refreshSidebar();
        if (data.chat_id !== root?.dataset.activeChat) return;
        if (message.type === 'message_created') scheduleRefresh();
        if (message.type === 'message_edited') {
          const row = document.getElementById(`message-${data.id}`);
          const text = row?.querySelector('[data-message-content]');
          if (text) { text.textContent = data.content; row.querySelector('[data-edited]')?.classList.remove('hidden'); }
        }
        if (message.type === 'message_deleted') document.getElementById(`message-${data.id}`)?.remove();
      });
      ws.addEventListener('close', () => {
        if (socket === ws) socket = null;
        connection(false);
        if (root) reconnectTimer = setTimeout(connect, retry);
        retry = Math.min(retry * 2, 30000);
      });
      ws.addEventListener('error', () => ws.close());
    } catch (_) {
      connection(false);
      if (root) reconnectTimer = setTimeout(connect, retry);
      retry = Math.min(retry * 2, 30000);
    } finally { connecting = false; }
  }
  function init() {
    const next = $('#app-root');
    if (root !== next) {
      controller.abort(); controller = new AbortController();
      root = next; lastRead = ''; pending = false; reconcile = false;
      if (root) {
        document.title = `${root.dataset.title || 'Nero'} — Nero`;
        const draft = drafts.get(root.dataset.activeChat), textarea = $('[data-composer]');
        if (draft && textarea && !textarea.value) {
          textarea.value = draft.content;
          $('[name="client_message_id"]').value = draft.id;
        }
        $('#message-feed')?.addEventListener('scroll', () => { if (nearBottom()) markRead(); }, { passive: true });
        requestAnimationFrame(scrollBottom);
      }
    }
    resize($('[data-composer]'));
    observeComposer();
    const seen = new Set();
    document.querySelectorAll('#chat-list [data-chat-id]').forEach(row => {
      if (seen.has(row.dataset.chatId)) row.remove();
      else seen.add(row.dataset.chatId);
    });
    filterChats();
    if (root) { connection(socket?.readyState === WebSocket.OPEN); connect(); }
    else { clearTimeout(reconnectTimer); socket?.close(); }
    const notice = $('#app-toast');
    if (notice?.textContent.trim()) { notice.hidden = false; clearTimeout(toastTimer); toastTimer = setTimeout(() => { notice.hidden = true; }, 6000); }
  }
  function editMessage(button) {
    const id = button.dataset.editMessage, row = document.getElementById(`message-${id}`), chat = root?.dataset.activeChat;
    if (!row || !chat) return;
    row.querySelector('details')?.removeAttribute('open');
    const form = document.createElement('form');
    form.id = row.id; form.className = 'message-edit page-enter'; form.method = 'post';
    form.action = `/app/chats/${chat}/messages/${id}/edit`;
    form.setAttribute('hx-post', form.action); form.setAttribute('hx-target', `#${form.id}`); form.setAttribute('hx-swap', 'outerHTML');
    const label = document.createElement('label'); label.textContent = 'Изменить сообщение'; label.htmlFor = `edit-${id}`;
    const input = document.createElement('textarea'); input.id = label.htmlFor; input.name = 'content'; input.className = 'auth-input'; input.maxLength = 4096; input.required = true; input.value = row.querySelector('[data-message-content]').textContent;
    const actions = document.createElement('div'); actions.className = 'form-actions';
    const cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'button-secondary'; cancel.textContent = 'Отмена';
    cancel.addEventListener('click', () => { form.replaceWith(row); htmx.process(row); if (pending) scheduleRefresh(); });
    const save = document.createElement('button'); save.type = 'submit'; save.className = 'button-primary'; save.textContent = 'Сохранить';
    actions.append(cancel, save); form.append(label, input, actions); row.replaceWith(form); htmx.process(form); input.focus();
  }
  document.addEventListener('input', event => {
    if (event.target.matches('[data-chat-search]')) filterChats();
    if (event.target.matches('[data-composer]')) {
      resize(event.target);
      drafts.set(root.dataset.activeChat, { content: event.target.value, id: $('[name="client_message_id"]').value });
    }
  });
  document.addEventListener('keydown', event => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing && event.target.matches('[data-composer]')) {
      event.preventDefault();
      if (event.target.value.trim() && !event.target.form.querySelector('button').disabled) event.target.form.requestSubmit();
    }
    if (event.key === 'Escape') document.querySelectorAll('details[open]').forEach(node => node.removeAttribute('open'));
  });
  document.addEventListener('click', event => {
    if (event.target.closest('[data-new-messages]')) scrollBottom();
    const edit = event.target.closest('[data-edit-message]');
    if (edit) editMessage(edit);
    document.querySelectorAll('details[open]').forEach(node => { if (!node.contains(event.target)) node.removeAttribute('open'); });
  });
  document.addEventListener('submit', event => {
    const text = event.target.dataset.confirm;
    if (text && !confirm(text)) { event.preventDefault(); event.stopImmediatePropagation(); }
  }, true);
  document.addEventListener('nero:message-sent', () => {
    drafts.delete(root?.dataset.activeChat); scrollBottom(); scheduleRefresh();
  });
  document.addEventListener('htmx:before:request', event => {
    const source = eventSource(event);
    if (source?.matches('[data-message-form]')) {
      source.querySelector('button').disabled = true;
      source.querySelector('textarea').readOnly = true;
    }
    if (source?.closest('#older-messages')) {
      const feed = $('#message-feed');
      event.detail.ctx.neroScroll = { feed, height: feed.scrollHeight, top: feed.scrollTop };
    }
  });
  document.addEventListener('htmx:after:swap', event => {
    const position = event.detail?.ctx?.neroScroll;
    if (position?.feed.isConnected) position.feed.scrollTop = position.top + position.feed.scrollHeight - position.height;
    init();
    if (eventSource(event)?.matches('[data-message-form]')) $('[data-composer]')?.focus();
    if (pending && !$('.message-edit')) scheduleRefresh();
  });
  document.addEventListener('htmx:after:process', () => {
    if (root !== $('#app-root') || observedComposer !== $('#composer')) init();
  });
  document.addEventListener('htmx:finally:request', event => {
    const source = eventSource(event);
    if (source?.matches('[data-message-form]')) {
      source.querySelector('button').disabled = false;
      source.querySelector('textarea').readOnly = false;
    }
  });
  document.addEventListener('htmx:error', event => {
    if (eventSource(event)?.closest('#app-root')) toast('Нет ответа от сервера. Проверь соединение и повтори попытку.');
  });
  document.addEventListener('visibilitychange', () => { if (!document.hidden) { connect(); scheduleRefresh(true); markRead(); } });
  window.addEventListener('online', () => { clearTimeout(reconnectTimer); connect(); scheduleRefresh(true); });
  window.addEventListener('popstate', () => requestAnimationFrame(init));
  init();
})();
