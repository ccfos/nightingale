import _ from 'lodash';
import { GraphDataInterface, CounterInterface } from '../interface';

export function transformMsToS(ts: string) {
  return Number(ts.substring(0, ts.length - 3));
}

export function processComparison(comparison: number[]) {
  const newComparison = [0];
  _.each(comparison, (o) => {
    newComparison.push(transformMsToS(String(o)));
  });
  return newComparison;
}

export default function normalizeEndpointCounters(graphConfig: GraphDataInterface, counterList: CounterInterface[]) {
  const firstMetric = _.get(graphConfig, 'metrics[0]', {});
  const { aggrFunc, aggrGroup: groupKey, consolFunc } = firstMetric;
  const start = transformMsToS(_.toString(graphConfig.start));
  const end = transformMsToS(_.toString(graphConfig.end));

  const counters = _.map(counterList, (counter) => {
    return {
      ...counter,
      start,
      end,
      aggrFunc,
      groupKey,
      consolFunc,
    };
  });

  return counters;
}
