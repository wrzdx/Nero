(() => {
  'use strict';
  const initialized = new WeakSet();
  const normalize = name => name.trim().replace(/^@/, '').toLowerCase();
  const initials = name => Array.from(name).slice(0, 2).join('').toUpperCase();

  function initPicker(picker) {
    if (initialized.has(picker)) return;
    initialized.add(picker);
    const input = picker.querySelector('[data-picker-query]');
    const selectedArea = picker.querySelector('[data-picker-selected]');
    const results = picker.querySelector('[data-picker-results]');
    const status = picker.querySelector('[data-picker-status]');
    const multiple = picker.dataset.multiple === 'true';
    const required = picker.dataset.required === 'true';
    const field = multiple ? 'participant_username' : 'peer_username';
    const excluded = new Set((picker.dataset.excluded || '').split(',').filter(Boolean).map(normalize));
    const selected = new Map();
    let timer, controller, revision = 0;
    input.removeAttribute('name');
    input.required = false;
    selectedArea.querySelectorAll('[data-selected-username]').forEach(chip => {
      const username = chip.dataset.selectedUsername;
      selected.set(normalize(username), { username });
    });

    function closeResults() {
      results.hidden = true;
      input.setAttribute('aria-expanded', 'false');
    }
    function hint(message) { status.textContent = message; }
    function renderSelected() {
      selectedArea.replaceChildren();
      for (const user of selected.values()) {
        const chip = document.createElement('span');
        chip.className = 'participant-chip';
        const avatar = document.createElement('span');
        avatar.className = 'chip-avatar'; avatar.textContent = initials(user.first_name || user.username);
        const name = document.createElement('span'); name.textContent = `@${user.username}`;
        const remove = document.createElement('button');
        remove.type = 'button'; remove.textContent = '×'; remove.setAttribute('aria-label', `Убрать @${user.username}`);
        remove.addEventListener('click', () => {
          selected.delete(normalize(user.username)); renderSelected();
          hint('Участник убран. Можно выбрать другого.'); input.focus();
          if (input.value.trim()) scheduleSearch();
        });
        const hidden = document.createElement('input'); hidden.type = 'hidden'; hidden.name = field; hidden.value = user.username;
        chip.append(avatar, name, remove, hidden); selectedArea.append(chip);
      }
      picker.classList.toggle('has-selection', selected.size > 0);
    }
    function choose(user) {
      const key = normalize(user.username);
      if (excluded.has(key) || selected.has(key)) return;
      if (multiple && selected.size >= 100) { hint('Не более 100 участников за один раз.'); return; }
      if (!multiple) selected.clear();
      selected.set(key, user); renderSelected();
      input.value = ''; clearTimeout(timer); controller?.abort(); revision++;
      closeResults(); hint(multiple ? `Выбрано: ${selected.size}. Можно добавить ещё.` : 'Собеседник выбран. Можно начать разговор.');
      input.focus();
    }
    function showUsers(users) {
      results.replaceChildren();
      if (!users.length) { closeResults(); hint('Никого не нашли. Проверь username.'); return; }
      for (const user of users) {
        const button = document.createElement('button');
        button.type = 'button'; button.className = 'picker-result';
        const avatar = document.createElement('span'); avatar.className = 'avatar'; avatar.textContent = initials(user.first_name);
        const text = document.createElement('span'); text.className = 'picker-result-person';
        const name = document.createElement('strong'); name.textContent = [user.first_name, user.last_name].filter(Boolean).join(' ');
        const username = document.createElement('small'); username.textContent = `@${user.username}`;
        text.append(name, username);
        const action = document.createElement('span'); action.className = 'picker-result-action';
        const key = normalize(user.username);
        button.disabled = excluded.has(key) || selected.has(key);
        action.textContent = excluded.has(key) ? 'Уже в группе' : selected.has(key) ? 'Выбран' : '+';
        button.append(avatar, text, action);
        button.addEventListener('click', () => choose(user)); results.append(button);
      }
      results.hidden = false; input.setAttribute('aria-expanded', 'true');
      hint('Выбери пользователя из списка.');
    }
    async function search() {
	  if (!picker.isConnected) return;
      const query = normalize(input.value), current = ++revision;
      controller?.abort(); controller = new AbortController();
      if (query.length < 2) { closeResults(); hint('Введи хотя бы 2 символа username.'); return; }
      if (!/^[a-z0-9_]{2,32}$/.test(query)) { closeResults(); hint('Для поиска: латиница, цифры и _.'); return; }
      closeResults(); hint('Ищем…');
      try {
        const response = await fetch(`/app/users/search?q=${encodeURIComponent(query)}`, { credentials: 'same-origin', cache: 'no-store', signal: controller.signal });
        const redirect = response.headers.get('HX-Redirect');
        if (redirect || response.redirected) { window.location.assign(redirect || response.url); return; }
        if (!response.ok) throw new Error('search failed');
        const data = await response.json();
        if (current !== revision || !picker.isConnected) return;
        showUsers(data.users);
      } catch (error) {
        if (error.name === 'AbortError' || current !== revision || !picker.isConnected) return;
        closeResults(); hint('Поиск недоступен. Попробуй ещё раз.');
        const retry = document.createElement('button'); retry.type = 'button'; retry.className = 'picker-retry'; retry.textContent = 'Повторить поиск';
        retry.addEventListener('click', search); results.replaceChildren(retry); results.hidden = false;
      }
    }
    function scheduleSearch() {
      clearTimeout(timer); controller?.abort(); revision++;
      closeResults(); hint(input.value.trim() ? 'Ищем…' : 'Введи хотя бы 2 символа username.');
      timer = setTimeout(search, 250);
    }
    input.addEventListener('input', event => { if (!event.isComposing) scheduleSearch(); });
    input.addEventListener('compositionend', scheduleSearch);
    input.addEventListener('search', scheduleSearch);
    input.addEventListener('focus', event => {
      if (!picker.contains(event.relatedTarget) && normalize(input.value).length >= 2) scheduleSearch();
    });
    picker.addEventListener('keydown', event => {
      const buttons = Array.from(results.querySelectorAll('button:not(:disabled)'));
      if (event.key === 'Escape') { clearTimeout(timer); controller?.abort(); revision++; closeResults(); input.focus(); }
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        if (!buttons.length || results.hidden) return;
        event.preventDefault();
        const index = buttons.indexOf(document.activeElement);
        const next = index < 0 ? (event.key === 'ArrowDown' ? 0 : buttons.length - 1) : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length;
        buttons[next].focus();
      }
      if (event.key === 'Enter' && event.target === input && !event.isComposing) {
        event.preventDefault();
        if (!results.hidden && buttons.length) buttons[0].click(); else search();
      }
    });
    picker.addEventListener('focusout', () => {
      setTimeout(() => { if (!picker.contains(document.activeElement)) closeResults(); }, 0);
    });
    picker.closest('form').addEventListener('submit', event => {
      if ((required && selected.size === 0) || input.value.trim()) {
        event.preventDefault(); event.stopImmediatePropagation();
        hint(input.value.trim() ? 'Выбери найденного пользователя или очисти поиск.' : 'Сначала выбери пользователя.'); input.focus();
      }
    }, true);
    renderSelected();
  }
  const init = () => document.querySelectorAll('[data-user-picker]').forEach(initPicker);
  document.addEventListener('htmx:after:swap', init);
  // outerHTML may detach the request source before after:swap can bubble.
  document.addEventListener('htmx:after:process', init);
  window.addEventListener('popstate', () => requestAnimationFrame(init));
  init();
})();
