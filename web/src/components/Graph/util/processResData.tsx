import _ from 'lodash';

export default function processResData(data: any[]) {
  removeEmptyVal(data);

  _.each(data, (item) => {
    item.values = _.map(item.values, (o) => {
      // convert from Unix timestamp to JavaScript time
      return [o.timestamp * 1000, o.value];
    });
  });
  return data;
}

function removeEmptyVal(data: any[]) {
  _.remove(data, (o) => {
    if (o && _.isArray(o.values) && o.values.length) {
      return false;
    }
    return true;
  });
}
