// Local interactions only. Requests and HTML updates are handled by htmx.
function requestForm(event) {
  // After a swap, htmx can emit an event on document for a detached form.
  const source = event.detail?.ctx?.sourceElement ?? event.target;
  return source instanceof Element ? source.closest("[data-auth-form]") : null;
}

document.addEventListener("click", (event) => {
  const button = event.target.closest("[data-password-toggle]");
  if (!button) return;
  const input = document.getElementById(button.getAttribute("aria-controls"));
  const visible = input.type === "password";
  input.type = visible ? "text" : "password";
  button.setAttribute("aria-pressed", String(visible));
  button.setAttribute("aria-label", visible ? "Скрыть пароль" : "Показать пароль");
  button.querySelector("[data-eye-slash]").classList.toggle("hidden", !visible);
});

document.addEventListener("htmx:before:request", (event) => {
  const form = requestForm(event);
  if (!form) return;
  form.querySelector('[type="submit"]').disabled = true;
  form.setAttribute("aria-busy", "true");
  form.querySelector(".auth-network-error").classList.add("hidden");
});

document.addEventListener("htmx:finally:request", (event) => {
  const form = requestForm(event);
  if (!form) return;
  form.querySelector('[type="submit"]').disabled = false;
  form.removeAttribute("aria-busy");
});

document.addEventListener("htmx:error", (event) => {
  const form = requestForm(event);
  if (!form) return;
  form.querySelector(".auth-network-error").classList.remove("hidden");
  form.querySelector('[type="submit"]').disabled = false;
  form.removeAttribute("aria-busy");
});
