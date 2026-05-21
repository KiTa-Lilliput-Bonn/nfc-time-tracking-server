/**
 * PrimeVue `Select` with `filter` renders a filter `<input>` inside an overlay.
 * This helper focuses that input right after the dropdown is shown.
 *
 * Usage: <Select ... filter @show="focusSelectFilterOnShow" />
 */
export function focusSelectFilterOnShow(): void {
  // Wait for overlay DOM to mount.
  setTimeout(() => {
    const candidates = Array.from(
      document.querySelectorAll<HTMLInputElement>(
        [
          // PrimeVue Select filter input (common)
          'input.p-select-filter',
          // Fallbacks for different PrimeVue/overlay wrappers
          '.p-select-overlay input[type="text"]',
          '.p-select-panel input[type="text"]',
          '.p-overlaypanel input[type="text"]',
        ].join(','),
      ),
    ).filter((el) => !el.disabled && el.offsetParent !== null)

    const el = candidates[candidates.length - 1]
    if (!el) return
    el.focus()
    try {
      el.select()
    } catch {
      /* ignore */
    }
  }, 0)
}

/**
 * Opens a PrimeVue Select (best-effort) and triggers filter focus via @show handler.
 * Useful when a parent Dialog just opened and the user should immediately search.
 */
export function openSelectDropdown(selectRef: any): void {
  if (!selectRef) return
  // PrimeVue components often expose show()/hide(). Use it when available.
  try {
    if (typeof selectRef.show === 'function') {
      selectRef.show()
      return
    }
  } catch {
    /* ignore */
  }
  // Fallback: click the root element.
  try {
    const el = (selectRef.$el ?? selectRef) as HTMLElement | undefined
    el?.click()
  } catch {
    /* ignore */
  }
}

