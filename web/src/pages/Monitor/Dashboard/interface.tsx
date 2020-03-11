export type GraphData = any;
export type Hosts = string[];
export type DynamicHostsType = '=all' | '=+' | '=-';
export type UpdateType = 'push' | 'unshift' | 'update' | 'allUpdate' | 'delete';
export type GraphId = string | number;
export interface SubclassData {
  id: number,
  name: string,
}
export type FilterMetricsType = 'prefix' | 'substring' | 'suffix' | undefined;
