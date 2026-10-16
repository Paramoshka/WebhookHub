(() => {
  if (window.WebhookHubDatePicker || !HTMLElement.prototype.showPopover) return;
  window.WebhookHubDatePicker = true;

  const monthFormat = new Intl.DateTimeFormat("en-GB", { month: "long", year: "numeric", timeZone: "UTC" });
  const dayFormat = new Intl.DateTimeFormat("en-GB", { dateStyle: "full", timeZone: "UTC" });
  let popup;
  let field;
  let trigger;
  let focusedDate;
  let selectedDate;

  // Calendar arithmetic uses UTC only as a wall-clock container. The existing
  // filter handler interprets the resulting input in the selected timezone.
  function calendarDate(year, month, day) {
    const date = new Date(0);
    date.setUTCFullYear(year, month, day);
    return date;
  }

  function dateKey(date) {
    return `${String(date.getUTCFullYear()).padStart(4, "0")}-${String(date.getUTCMonth() + 1).padStart(2, "0")}-${String(date.getUTCDate()).padStart(2, "0")}`;
  }

  function position() {
    if (!popup?.matches(":popover-open") || !field?.isConnected) return;
    const rect = field.getBoundingClientRect();
    const left = Math.max(12, Math.min(rect.left, innerWidth - popup.offsetWidth - 12));
    let top = rect.bottom + 8;
    if (top + popup.offsetHeight > innerHeight - 12) top = rect.top - popup.offsetHeight - 8;
    top = Math.max(12, Math.min(top, innerHeight - popup.offsetHeight - 12));
    popup.style.left = `${left}px`;
    popup.style.top = `${top}px`;
  }

  function renderDays(focusDay = false) {
    const year = focusedDate.getUTCFullYear();
    const month = focusedDate.getUTCMonth();
    const first = calendarDate(year, month, 1);
    const offset = (first.getUTCDay() + 6) % 7;
    const days = document.createDocumentFragment();
    const today = window.WebhookHubTimezone.currentDateTime().split("T")[0];
    document.getElementById("date-picker-month").textContent = monthFormat.format(first);
    for (let week = 0; week < 6; week++) {
      const row = document.createElement("tr");
      for (let day = 0; day < 7; day++) {
        const date = calendarDate(year, month, 1 - offset + week * 7 + day);
        const key = dateKey(date);
        const cell = document.createElement("td");
        cell.setAttribute("aria-selected", String(key === selectedDate));
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = date.getUTCDate();
        button.dataset.calendarDay = key;
        button.setAttribute("aria-label", dayFormat.format(date));
        button.tabIndex = key === dateKey(focusedDate) ? 0 : -1;
        button.disabled = date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999;
        if (date.getUTCMonth() !== month) button.classList.add("date-picker-adjacent");
        if (key === today) button.setAttribute("aria-current", "date");
        cell.append(button);
        row.append(cell);
      }
      days.append(row);
    }
    document.getElementById("date-picker-days").replaceChildren(days);
    popup.querySelector('[data-calendar-action="previous"]').disabled = year === 1 && month === 0;
    popup.querySelector('[data-calendar-action="next"]').disabled = year === 9999 && month === 11;
    if (focusDay) popup.querySelector('[data-calendar-day][tabindex="0"]').focus({ preventScroll: true });
  }

  function close(restoreFocus = true) {
    if (!popup?.matches(":popover-open")) return;
    popup.hidePopover();
    trigger.setAttribute("aria-expanded", "false");
    if (restoreFocus && trigger.isConnected) trigger.focus({ preventScroll: true });
  }

  function open(input, button) {
    if (popup?.matches(":popover-open") && field === input) {
      close();
      return;
    }
    close(false);
    trigger?.setAttribute("aria-expanded", "false");
    popup = document.getElementById("date-picker-popup");
    field = input;
    trigger = button;
    const value = field.value || window.WebhookHubTimezone.currentDateTime().split("T")[0] + "T00:00:00";
    const [date, time] = value.split("T");
    const [year, month, day] = date.split("-").map(Number);
    focusedDate = calendarDate(year, month - 1, day);
    selectedDate = dateKey(focusedDate);
    document.getElementById("date-picker-time").value = time;
    document.getElementById("date-picker-context").textContent = `${field.name === "from" ? "From" : "To"} · ${field.form.elements.tz.value}`;
    renderDays();
    popup.showPopover();
    trigger.setAttribute("aria-expanded", "true");
    position();
    popup.querySelector('[data-calendar-day][tabindex="0"]').focus({ preventScroll: true });
  }

  function changeMonth(delta) {
    const year = focusedDate.getUTCFullYear();
    const month = focusedDate.getUTCMonth() + delta;
    const day = Math.min(focusedDate.getUTCDate(), calendarDate(year, month + 1, 0).getUTCDate());
    return calendarDate(year, month, day);
  }

  function commit(clear = false) {
    const time = document.getElementById("date-picker-time");
    if (!clear && !time.reportValidity()) return;
    field.value = clear ? "" : `${selectedDate}T${time.value}`;
    field.dispatchEvent(new Event("input", { bubbles: true }));
    field.dispatchEvent(new Event("change", { bubbles: true }));
    close();
  }

  function handleClick(event) {
    const day = event.target.closest("[data-calendar-day]");
    if (day) {
      const [year, month, date] = day.dataset.calendarDay.split("-").map(Number);
      focusedDate = calendarDate(year, month - 1, date);
      selectedDate = dateKey(focusedDate);
      renderDays(true);
      return;
    }
    const action = event.target.closest("[data-calendar-action]")?.dataset.calendarAction;
    if (action === "clear" || action === "done") commit(action === "clear");
    else if (action === "previous" || action === "next") {
      focusedDate = changeMonth(action === "previous" ? -1 : 1);
      renderDays();
    }
  }

  function handleKeydown(event) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
      return;
    }
    if (event.key === "Enter" && event.target.id === "date-picker-time") {
      event.preventDefault();
      commit();
      return;
    }
    if (!event.target.matches("[data-calendar-day]")) return;
    let date;
    const weekday = (focusedDate.getUTCDay() + 6) % 7;
    const offsets = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7, Home: -weekday, End: 6 - weekday };
    if (event.key in offsets) date = calendarDate(focusedDate.getUTCFullYear(), focusedDate.getUTCMonth(), focusedDate.getUTCDate() + offsets[event.key]);
    else if (event.key === "PageUp" || event.key === "PageDown") date = changeMonth((event.key === "PageUp" ? -1 : 1) * (event.shiftKey ? 12 : 1));
    else return;
    event.preventDefault();
    if (date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999) return;
    focusedDate = date;
    renderDays(true);
  }

  function initialize() {
    const picker = document.getElementById("date-picker-popup");
    if (!picker) return;
    if (!picker.dataset.ready) {
      picker.dataset.ready = "true";
      picker.addEventListener("click", handleClick);
      picker.addEventListener("keydown", handleKeydown);
      picker.addEventListener("toggle", (event) => {
        if (event.target === popup && event.newState === "closed") trigger?.setAttribute("aria-expanded", "false");
      });
    }
    for (const wrapper of document.querySelectorAll(".date-picker-field")) {
      if (wrapper.classList.contains("has-calendar")) continue;
      const input = wrapper.querySelector("input");
      const button = wrapper.querySelector("button");
      button.addEventListener("click", () => open(input, button));
      input.addEventListener("keydown", (event) => {
        if (event.altKey && event.key === "ArrowDown") {
          event.preventDefault();
          open(input, button);
        }
      });
      wrapper.classList.add("has-calendar");
      button.hidden = false;
    }
  }

  document.addEventListener("change", (event) => {
    if (event.target.id === "timezone-select") close(false);
  });
  document.addEventListener("htmx:beforeSwap", (event) => {
    if (event.detail.shouldSwap && field && event.detail.target.contains(field)) close(false);
  });
  window.addEventListener("resize", position);
  document.addEventListener("scroll", position, true);
  document.addEventListener("htmx:load", initialize);
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", initialize);
  else initialize();
})();
