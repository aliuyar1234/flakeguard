function getCsrfToken(form) {
  const csrf = form.querySelector('input[name="_csrf"]');
  return csrf ? csrf.value : '';
}

function showFormMessage(form, kind, message) {
  const className = kind === 'success' ? 'success' : 'error';

  let el = form.querySelector(`.${className}`);
  if (!el) {
    el = document.createElement('div');
    el.className = className;
    form.prepend(el);
  }

  el.textContent = message;
}

function setFormSubmitting(form, submitting) {
  form.querySelectorAll('button[type="submit"], input[type="submit"]').forEach((btn) => {
    btn.disabled = submitting;
  });
}

function getEffectiveMethod(form) {
  const override = form.querySelector('input[name="_method"]')?.value?.trim();
  if (override) return override.toUpperCase();

  const method = (form.getAttribute('method') || 'POST').trim();
  return method.toUpperCase();
}

function serializeFormToJson(form) {
  const formData = new FormData(form);
  const payload = {};

  for (const [key, value] of formData.entries()) {
    if (key === '_csrf' || key === '_method') continue;

    if (Object.prototype.hasOwnProperty.call(payload, key)) {
      const existing = payload[key];
      if (Array.isArray(existing)) {
        existing.push(value);
      } else {
        payload[key] = [existing, value];
      }
    } else {
      payload[key] = value;
    }
  }

  const checkboxGroups = new Map();
  form.querySelectorAll('input[type="checkbox"][name]').forEach((el) => {
    if (el.disabled) return;
    const name = el.getAttribute('name');
    if (!checkboxGroups.has(name)) checkboxGroups.set(name, []);
    checkboxGroups.get(name).push(el);
  });

  for (const [name, boxes] of checkboxGroups.entries()) {
    if (boxes.length === 1) {
      payload[name] = boxes[0].checked;
      continue;
    }

    const existing = payload[name];
    if (existing === undefined) {
      payload[name] = [];
    } else if (!Array.isArray(existing)) {
      payload[name] = [existing];
    }
  }

  form.querySelectorAll('input[type="number"][name]').forEach((el) => {
    if (el.disabled) return;
    const name = el.getAttribute('name');
    const v = payload[name];
    if (v === undefined || v === '') return;
    const num = Number(v);
    if (!Number.isNaN(num)) payload[name] = num;
  });

  return payload;
}

async function submitJsonForm(form) {
  const method = getEffectiveMethod(form);
  if (method === 'GET' || method === 'HEAD') return;

  const csrfToken = getCsrfToken(form);
  const payload = serializeFormToJson(form);

  setFormSubmitting(form, true);
  try {
    const headers = {
      Accept: 'application/json',
      'X-CSRF-Token': csrfToken,
    };

    const init = { method, headers };
    if (method !== 'DELETE') {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(payload ?? {});
    }

    const res = await fetch(form.action, init);

    let body = null;
    try {
      body = await res.json();
    } catch {
      // ignore
    }

    if (!res.ok) {
      const message = body?.error?.message || `Request failed (${res.status})`;
      showFormMessage(form, 'error', message);
      return;
    }

    const tokenTarget = form.dataset.tokenTarget;
    if (tokenTarget) {
      const token = body?.data?.api_key?.token;
      if (token) {
        const el = document.querySelector(tokenTarget);
        if (el) el.textContent = token;

        const reveal = form.dataset.tokenReveal;
        if (reveal) {
          const revealEl = document.querySelector(reveal);
          if (revealEl) revealEl.classList.remove('hidden');
        }
      }
    }

    const redirectTo = form.dataset.redirect;
    if (redirectTo) {
      window.location.href = redirectTo;
      return;
    }

    if (form.dataset.reload === 'true') {
      window.location.reload();
      return;
    }

    showFormMessage(form, 'success', 'Saved.');
  } catch (err) {
    showFormMessage(form, 'error', err?.message || 'Network error');
  } finally {
    setFormSubmitting(form, false);
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.querySelectorAll('form[data-json-form]').forEach((form) => {
    form.addEventListener('submit', (e) => {
      e.preventDefault();

      const confirmMessage = form.dataset.confirm;
      if (confirmMessage && !window.confirm(confirmMessage)) return;

      submitJsonForm(form);
    });
  });
});
