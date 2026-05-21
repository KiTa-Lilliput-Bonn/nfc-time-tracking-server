/** Mirrors backend model.ActorMayManageUser: Leitung must not manage superadmin accounts. */
export function canManageEmployeeByRole(
  actorRole: string | null | undefined,
  targetRole: string,
): boolean {
  if (!actorRole) return false
  if (targetRole === 'superadmin') return actorRole === 'superadmin'
  return true
}
