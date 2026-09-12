import { useMemo } from 'react'
import type { OrganizerGroup } from '../api/client'
import SelectMenu from './SelectMenu'

type FilterValue = 'all' | 'ungrouped' | string

export default function GroupFilter({ groups, value, counts, onChange }: { groups: OrganizerGroup[]; value: FilterValue; counts: Record<string, number>; onChange: (value: FilterValue) => void }) {
  const options = useMemo(() => [{ value: 'all', label: '全部邮箱', count: counts.all ?? 0 }, { value: 'ungrouped', label: '未分组', count: counts.ungrouped ?? 0 }, ...groups.map((group) => ({ value: group.id, label: group.name, count: counts[group.id] ?? 0 }))], [groups, counts])
  return <SelectMenu value={value} options={options} onChange={onChange} ariaLabel="按分组筛选别名" className="group-filter-menu" />
}
