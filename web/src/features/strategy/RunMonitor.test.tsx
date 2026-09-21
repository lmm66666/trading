import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { cancelRun, type RunStatus } from '../../api/client'
import { RunMonitor } from './RunMonitor'

vi.mock('../../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api/client')>()),
  cancelRun: vi.fn(),
}))

const running: RunStatus = {
  run_id: 'run-abc123',
  status: 'RUNNING',
  kind: 'scan',
  strategy: 'daily_b1_buy',
  strategy_version: '1',
  data_version: 7,
  engine_version: 'v1',
  attempts: 2,
  cancel_requested_at: null,
}

describe('RunMonitor', () => {
  it('状态条以徽标与策略版本为主，默认不展示完整 run_id', () => {
    render(<RunMonitor kind="scan" status={running} pollingError={null} />)
    expect(screen.getByText('运行中')).toBeVisible()
    expect(screen.getByText('daily_b1_buy v1')).toBeVisible()
    expect(screen.getByText('run-abc123')).not.toBeVisible()
  })

  it('展开任务详情显示 run_id、数据版本与尝试次数', () => {
    render(<RunMonitor kind="scan" status={running} pollingError={null} />)
    fireEvent.click(screen.getByText('任务详情'))
    const details = screen.getByText('任务详情').closest('details') as HTMLElement
    expect(within(details).getByText('run-abc123')).toBeVisible()
    expect(within(details).getByText('数据版本')).toBeVisible()
    expect(within(details).getByText('7')).toBeVisible()
    expect(within(details).getByText('尝试次数')).toBeVisible()
    expect(within(details).getByText('2')).toBeVisible()
  })

  it('已请求取消时任务详情显示取消标记', () => {
    render(
      <RunMonitor
        kind="scan"
        status={{ ...running, cancel_requested_at: '2026-06-01T00:00:00Z' }}
        pollingError={null}
      />,
    )
    fireEvent.click(screen.getByText('任务详情'))
    const details = screen.getByText('任务详情').closest('details') as HTMLElement
    expect(within(details).getByText('已请求取消')).toBeVisible()
  })

  it('可取消状态下取消按钮保持在卡头可见', async () => {
    vi.mocked(cancelRun).mockResolvedValue(undefined)
    render(<RunMonitor kind="scan" status={running} pollingError={null} />)
    const button = screen.getByRole('button', { name: '取消任务' })
    expect(button).toBeVisible()
    fireEvent.click(button)
    await vi.waitFor(() => expect(cancelRun).toHaveBeenCalledWith('scan', 'run-abc123'))
  })

  it('终态不显示取消按钮', () => {
    render(<RunMonitor kind="scan" status={{ ...running, status: 'SUCCEEDED' }} pollingError={null} />)
    expect(screen.queryByRole('button', { name: '取消任务' })).toBeNull()
  })

  it('无状态且轮询异常时显示错误徽标与错误信息', () => {
    render(<RunMonitor kind="scan" status={null} pollingError="network down" />)
    expect(screen.getByText('轮询异常')).toBeVisible()
    expect(screen.getByText('network down')).toBeVisible()
  })

  it('无状态且无错误时不渲染', () => {
    const { container } = render(<RunMonitor kind="scan" status={null} pollingError={null} />)
    expect(container).toBeEmptyDOMElement()
  })
})
