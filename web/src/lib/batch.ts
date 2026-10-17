// runOnEach runs op on every id, one at a time, and returns the ids it failed
// on, so a batch can say how much of it did not happen.
export async function runOnEach(ids: string[], op: (id: string) => Promise<unknown>): Promise<string[]> {
  const failed: string[] = []
  for (const id of ids) {
    try {
      await op(id)
    } catch {
      failed.push(id)
    }
  }
  return failed
}
