import _ from 'lodash';
import { FilterMetricsType, GraphData } from './interface';

export function filterMetrics(filterType: FilterMetricsType, filterValue: string, alls: any[]) {
  if (!(filterType && filterValue)) {
    return [];
  }

  const filters = _.split(filterValue, ',');
  switch (filterType) {
    case 'prefix':
      return _.filter(alls, (m) => {
        for (let i = 0; i < filters.length; i++) {
          if (m && m.indexOf(filters[i]) === 0) {
            return true;
          }
        }
        return false;
      });
    case 'substring':
      return _.filter(alls, (m) => {
        for (let i = 0; i < filters.length; i++) {
          if (m && m.indexOf(filters[i]) !== -1) {
            return true;
          }
        }
        return false;
      });
    case 'suffix':
      return _.filter(alls, (m) => {
        for (let i = 0; i < filters.length; i++) {
          if (m && m.indexOf(filters[i], m.length - filters[i].length) !== -1) {
            return true;
          }
        }
        return false;
      });
    default:
      return [];
  }
}

export function matchMetrics(matches: any[], alls: any[]) {
  if (!matches || matches.length === 0) {
    return [];
  }
  if (!alls || alls.length === 0) {
    return [];
  }

  return _.filter(matches, o => _.indexOf(alls, o) > -1);
}

export function getClusterNs(allNsData: string[], query: any) {
  const { ns, category } = query;
  if (category === 'service') {
    return _.filter(allNsData, (item) => {
      const nsSplit = item.split('.');
      nsSplit.splice(0, 2);
      return nsSplit.join('.') === ns;
    });
  }
  return [`collect.${ns}`];
}

export function normalizeGraphData(data: GraphData) {
  const cloneData = _.cloneDeep(data);
  _.each(cloneData.metrics, (item) => {
    delete item.key;
    delete item.metrics;
    delete item.tagkv;
    delete item.counterList;
    delete item.endpoints;
  });
  return cloneData;
}
