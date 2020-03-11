import _ from 'lodash';
import * as config from '../config';

export default function getTimeLabelVal(start: string, end: string, key: string) {
  const interval = Number(end) - Number(start);
  const currentTime = _.find(config.time, { value: _.toString(interval) });
  if (currentTime) {
    return currentTime[key];
  }
  return key === 'label' ? '自定义' : 'custom';
}
