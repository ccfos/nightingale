import _ from 'lodash';

export default function isEqualBy(value: any, other: any, keyName: string) {
  return _.isEqualWith(value, other, (objValue, othValue, index) => {
    if (index === undefined) return undefined;
    return _.isEqual(objValue[keyName], othValue[keyName]);
  });
}
