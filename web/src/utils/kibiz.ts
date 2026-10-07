import type { CareHours, ChildCount, Employee, GroupForm, KibizTotals, Qualification } from '@/types/api'

export const GROUP_FORMS: GroupForm[] = ['I', 'II', 'III']
export const CARE_HOURS: CareHours[] = [25, 35, 45]

export const GROUP_FORM_LABELS: Record<GroupForm, string> = {
  I: 'Gruppenform I (2 Jahre bis Schule)',
  II: 'Gruppenform II (unter 3)',
  III: 'Gruppenform III (ab 3)',
}

export const QUALIFICATION_LABELS: Record<Qualification, string> = {
  fachkraft: 'Fachkraft',
  ergaenzungskraft: 'Ergänzungskraft',
  leitung: 'Leitung (zählt nicht)',
  hauswirtschaft: 'Hauswirtschaft (zählt nicht)',
  sonstige: 'Sonstige (zählt nicht)',
}

/** Kraft einer Person wie in der Rechnung: Leitungskonten ohne Eintrag gelten als Leitung. */
export function effectiveQualification(
  e: Pick<Employee, 'id' | 'role'>,
  qualifications: Record<string | number, Qualification | undefined>,
): Qualification | undefined {
  return qualifications[e.id] ?? (e.role === 'leitung' ? 'leitung' : undefined)
}

/** „III · 35 h“ */
export function childCategoryLabel(c: Pick<ChildCount, 'group_form' | 'care_hours'>): string {
  return `${c.group_form} · ${c.care_hours} h`
}

export function childKey(c: Pick<ChildCount, 'group_form' | 'care_hours'>): string {
  return `${c.group_form}/${c.care_hours}`
}

export function childTotal(counts: ChildCount[]): number {
  return counts.reduce((s, c) => s + c.count, 0)
}

/** Sortiert nach Gruppenform und Betreuungszeit, ohne Nullzeilen. */
export function normalizeChildCounts(counts: ChildCount[]): ChildCount[] {
  return counts
    .filter((c) => c.count > 0)
    .sort((a, b) => GROUP_FORMS.indexOf(a.group_form) - GROUP_FORMS.indexOf(b.group_form) || a.care_hours - b.care_hours)
}

export function sameChildCounts(a: ChildCount[], b: ChildCount[]): boolean {
  const x = normalizeChildCounts(a)
  const y = normalizeChildCounts(b)
  return x.length === y.length && x.every((c, i) => childKey(c) === childKey(y[i]!) && c.count === y[i]!.count)
}

/** Fehlende Minuten (0 = genug): Fachkraft gegen Mindest-Fachkraftstunden, alle zusammen gegen Gesamtstunden. */
export function kibizShortfall(t: KibizTotals): { fachkraft: number; total: number } {
  return {
    fachkraft: Math.max(0, t.need_fachkraft_min - t.planned_fachkraft_min),
    total: Math.max(0, t.need_total_min - t.planned_fachkraft_min - t.planned_ergaenzung_min),
  }
}

/** Unter einer Viertelstunde Abweichung gilt als passend (Rundung, Pausen). */
export const KIBIZ_TOLERANCE_MIN = 15
