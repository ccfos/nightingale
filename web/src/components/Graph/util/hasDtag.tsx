import _ from 'lodash';
import { TagkvInterface } from '../interface';

const DtagKws = ['=all', '=+', '=-'];

/**
 * 是否包含动态tag
 */
export default function hasDtag(data: (TagkvInterface | string)[] = []) {
  return _.some(data, (item) => {
    if (_.isObject(item) && _.isArray(item.tagv)) {
      return _.some(item.tagv, (subItem) => {
        if (_.isString(subItem)) {
          return hasDtagByStrArr(subItem);
        }
        return false;
      });
    }
    if (_.isString(item)) {
      return hasDtagByStrArr(item);
    }
    return false;
  });
};

function hasDtagByStrArr(data: string) {
  return _.some(DtagKws, o => {
    return data.indexOf(o) === 0;
  });
}
