import moment from 'moment';
import _ from 'lodash';

/**
 * trim 结构体中所有的字符串类型的值
 * @return {Object} [description]
 */
export function objectTrim(object: any) {
  if (_.isArray(object)) {
    _.each(object, (item) => {
      objectTrim(item);
    });
  } else if (_.isPlainObject(object)) {
    _.each(object, (val, key) => {
      if (_.isString(val)) {
        object[key] = _.trim(val);
      } else if (_.isArray(val) && _.every(val, o => _.isString(o))) {
        object[key] = _.map(val, o => _.trim(o));
      } else if (_.isArray(val) || _.isPlainObject(val)) {
        objectTrim(val);
      }
    });
  }
}

/**
 * 待优化！！！！
 */
export function getProductNsPath(fullpath = '') {
  if (!fullpath || !fullpath.length) {
    return '';
  }

  const nses = _.split(fullpath, '.');
  const nssize = nses.length;
  if (nssize < 3) {
    return '';
  }

  // eslint-disable-next-line prefer-template
  return nses[nssize - 3] + '.' + nses[nssize - 2] + '.' + nses[nssize - 1];
}

export function normalizReqData(data: any) {
  const reqData = {
    btime: moment(data.btime).unix(),
    etime: moment(data.etime).unix(),
    cause: data.cause,
    metric: data.metric,
    tags: data.tags,
    endpoints: _.split(data.endpoints, '\n'),
  };

  return reqData;
}
