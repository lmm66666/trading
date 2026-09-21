import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ZScoreSummary } from './ZScoreSummary'
import type { ZScoreDiagnostic } from '../../api/client'
it('shows pair diagnostics, unavailable values and scoped errors', () => {
 const item: ZScoreDiagnostic = {key:'a', comparison:'SHFE:AU.MAIN',period:126,regime:252,lag:0,commodity_date:'2026-09-18',z:2.1,long_z:null,relative_performance:12.5,correlation:.85,state:'股票阶段性偏强'}
 const {rerender} = render(<ZScoreSummary item={item}/> )
 expect(screen.getByText(/126期 Z 2.10σ · 252期 Z —/)).toBeVisible()
 expect(screen.getByText(/63期相对表现 12.50%/)).toBeVisible()
 expect(screen.getByText(/0.85/)).toBeVisible()
 expect(screen.getByText(/2026-09-18/)).toBeVisible()
 rerender(<ZScoreSummary item={{...item,comparison:'',warning:'请选择看板关联期货'}}/> )
 expect(screen.getByRole('status')).toHaveTextContent('请选择看板关联期货')
 expect(screen.queryByText(/0.85/)).toBeNull()
 rerender(<ZScoreSummary item={{...item,commodity_date:''}}/> )
 expect(screen.getByText(/最近已知期货收盘价：—/)).toBeVisible()
})
