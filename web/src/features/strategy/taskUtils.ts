/** 共享的任务输入工具：日期窗口与错误文案 */

/** 把 <input type="date"> 的 YYYY-MM-DD 转换为 UTC 零点的 RFC3339 */
export function toRFC3339(date: string): string {
  return `${date}T00:00:00Z`
}

/** 校验日期窗口：两端必填、顺序正确、跨度不超过 20 年；返回错误文案或 null */
export function validateDateRange(from: string, to: string): string | null {
  if (!from || !to) return '请选择开始与截止日期'
  if (from > to) return '开始日期不能晚于截止日期'
  const fromDate = new Date(`${from}T00:00:00Z`)
  const limit = new Date(fromDate)
  limit.setUTCFullYear(limit.getUTCFullYear() + 20)
  if (new Date(`${to}T00:00:00Z`) > limit) return '时间跨度不能超过 20 年'
  return null
}

/** 把任务接口的稳定错误标识映射为可读文案，其余原样返回 */
export function describeTaskError(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error)
  if (message === 'IDEMPOTENCY_CONFLICT') return '任务输入与已有任务冲突，请重新提交'
  return message
}
