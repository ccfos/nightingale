import _ from 'lodash';
import { TagkvInterface } from '../interface';

export function dFilter(dType: string, firstTagv: string, currentTagv: string[]) {
  const dValue = firstTagv.replace(dType, '');
  const reg = new RegExp(dValue);
  return _.filter(currentTagv, (o) => {
    if (dType === '=all') {
      return true;
    }
    if (dType === '=+') {
      return reg.test(o);
    }
    if (dType === '=-') {
      return !reg.test(o);
    }
    return false;
  });
}

export default function getDTagV(tagkvs: TagkvInterface[], tagkv: TagkvInterface) {
  const { tagk, tagv = [''] } = tagkv;
  const currentTagkv = _.find(tagkvs, { tagk }) || {};
  const currentTagv = currentTagkv.tagv || [];
  let newTagv = tagv;
  const firstTagv = tagv[0] || '';
  if (firstTagv.indexOf('=all') === 0) {
    if (_.includes(currentTagv, '<all>')) {
      // 动态全选排除<all>
      newTagv = _.filter(currentTagv, o => o !== '<all>');
    } else {
      newTagv = currentTagv;
    }
  } else if (firstTagv.indexOf('=+') === 0) {
    newTagv = dFilter('=+', firstTagv, currentTagv);
  } else if (firstTagv.indexOf('=-') === 0) {
    newTagv = dFilter('=-', firstTagv, currentTagv);
  }
  return newTagv;
}
