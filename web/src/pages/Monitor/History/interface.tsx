export interface DataItem {
  id: number,
  etime: number,
  sname: string,
  priority: 1 | 2 | 3,
  endpoint: string,
  tags: string,
  status: string[],
}
