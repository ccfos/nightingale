/* eslint-disable no-plusplus */
import _ from 'lodash';
import request from '@common/request';
import api from '@common/api';
import commonApi from '@common/api';
import hasDtag from './util/hasDtag';
import getDTagV, { dFilter } from './util/getDTagV';
import processResData from './util/processResData';
import { DynamicHostsType, Endpoints } from './interface';

function getApi(key: string) {
  const api: { [index: string]: string } = {
    metrics: `${commonApi.graphIndex}/metrics`,
    tagkv: `${commonApi.graphIndex}/tagkv`,
    counter: `${commonApi.graphIndex}/counter/fullmatch`,
    history: `${commonApi.graphTransfer}/data/ui`,
  };
  return api[key];
}

function getDTagvKeyword(firstTagv: DynamicHostsType | string) {
  if (firstTagv === '=all') {
    return '=all';
  }
  if (firstTagv.indexOf('=+') === 0) {
    return '=+';
  }
  if (firstTagv.indexOf('=-') === 0) {
    return '=-';
  }
}

export function fetchEndPoints(nid: number) {
  return request(`${commonApi.endpoint}s/bynodeids?ids=${nid}`, undefined, false).then((data) => {
    return _.map(data, 'ident');
  });
}

export function fetchMetrics(selectedEndpoint: Endpoints, endpoints: Endpoints) {
  if (hasDtag(selectedEndpoint)) {
    const dTagvKeyword = getDTagvKeyword(selectedEndpoint[0]);
    selectedEndpoint = dFilter(dTagvKeyword, selectedEndpoint[0], endpoints);
  }
  return request(getApi('metrics'), {
    method: 'POST',
    body: JSON.stringify({
      endpoints: selectedEndpoint,
    }),
  }, false).then((data) => {
    return _.chain(data.metrics).compact().flattenDeep().union().sortBy((o) => {
      return _.lowerCase(o);
    }).value();
  });
}

export function fetchTagkv(selectedEndpoint: Endpoints, selectedMetric: string, endpoints: Endpoints) {
  if (hasDtag(selectedEndpoint)) {
    const dTagvKeyword = getDTagvKeyword(selectedEndpoint[0]);
    selectedEndpoint = dFilter(dTagvKeyword, selectedEndpoint[0], endpoints);
  }
  return request(getApi('tagkv'), {
    method: 'POST',
    body: JSON.stringify({
      endpoints: _.isArray(selectedEndpoint) ? selectedEndpoint : [selectedEndpoint],
      metrics: _.isArray(selectedMetric) ? selectedMetric : [selectedMetric],
    }),
  }, false).then((data) => {
    let allTagkv: any[] = [];
    _.each(data, (item) => {
      const { tagkv } = item;
      allTagkv = [
        {
          tagk: 'endpoint',
          tagv: endpoints,
        },
        ...tagkv || [],
      ];
    });
    return allTagkv;
  });
}

export function fetchCounter(queryBody: any) {
  return request(getApi('counter'), {
    method: 'POST',
    body: JSON.stringify(queryBody),
  }, false);
}

/**
 * 标准化 metrics 数据
 * 主要是补全 tagkv 和 设置默认 selectedTagkv
 */
export async function normalizeMetrics(metrics: any[], graphConfigInnerVisible: boolean) {
  const metricsClone = _.cloneDeep(metrics);
  let canUpdate = false;

  for (let m = 0; m < metricsClone.length; m++) {
    const { selectedEndpoint, selectedNid, selectedMetric, selectedTagkv, tagkv } = metricsClone[m];
    let { endpoints } = metricsClone[m];
    // 加载 tagkv 规则，满足
    // 开启行级配置 或者 包含动态tag 或者 没有选择tag
    if (
      _.isEmpty(tagkv) &&
      (!!graphConfigInnerVisible || hasDtag(selectedTagkv) || _.isEmpty(selectedTagkv))
    ) {
      canUpdate = true;
      if (hasDtag(selectedEndpoint)) {
        endpoints = await fetchEndPoints(selectedNid);
      }
      const newTagkv = await fetchTagkv(selectedEndpoint, selectedMetric, endpoints);
      metricsClone[m].tagkv = newTagkv;
      metricsClone[m].endpoints = endpoints;
      if (_.isEmpty(selectedTagkv)) {
        metricsClone[m].selectedTagkv = newTagkv;
      }
    }
  }
  return {
    metrics: metricsClone,
    canUpdate,
  };
}

export async function fetchCounterList(metrics: any[]) {
  const queryBody = [];

  for (let m = 0; m < metrics.length; m++) {
    const { selectedMetric, selectedTagkv, tagkv, endpoints } = metrics[m];
    let { selectedEndpoint } = metrics[m];

    if (hasDtag(selectedEndpoint)) {
      const dTagvKeyword = getDTagvKeyword(selectedEndpoint[0]);
      selectedEndpoint = dFilter(dTagvKeyword, selectedEndpoint[0], endpoints);
    }

    let newSelectedTagkv = selectedTagkv;

    // 动态tag场景
    if (hasDtag(selectedTagkv)) {
      newSelectedTagkv = _.map(newSelectedTagkv, (item) => {
        return {
          tagk: item.tagk,
          tagv: getDTagV(tagkv, item),
        };
      });
    }

    const excludeEndPoints = _.filter(newSelectedTagkv, (item) => {
      return item.tagk !== 'endpoint';
    });

    queryBody.push({
      endpoints: selectedEndpoint,
      metric: selectedMetric,
      tagkv: excludeEndPoints,
    });
  }

  // eslint-disable-next-line no-return-await
  return await fetchCounter(queryBody);
}

export function fetchHistory(endpointCounters: any[]) {
  return request(getApi('history'), {
    method: 'POST',
    body: JSON.stringify(endpointCounters),
  }, false).then((data) => {
    return processResData(data);
  });
}

export async function getHistory(endpointCounters: any[]) {
  let sourceData: any[] = [];
  let i = 0;
  for (i; i < endpointCounters.length; i++) {
    // eslint-disable-next-line no-await-in-loop
    const data = await fetchHistory(endpointCounters[i]);
    if (data) {
      sourceData = _.concat(sourceData, data);
    }
  }
  return sourceData;
}
