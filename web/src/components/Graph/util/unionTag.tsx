import _ from 'lodash';

export default function unionTag(data = []) {
  let unionTagkv = [] as any[];
  _.each(data, (dataItem) => {
    const { tagkv = [] } = dataItem;
    _.each(tagkv, (tagItem) => {
      const { tagk, tagv = [] } = tagItem;
      const tagvs = _.filter(tagv, (v) => { return v; });
      const currentTag = _.find(unionTagkv, { tagk });

      if (currentTag) {
        currentTag.tagv = _.sortBy(_.union(currentTag.tagv, tagvs));
      } else {
        unionTagkv.push({
          tagk,
          tagv: _.sortBy(tagvs),
        });
      }
    });
  });

  const hosts = _.remove(unionTagkv, (item) => {
    return item.tagk === 'host';
  });
  unionTagkv = _.sortBy(unionTagkv, 'tagk');
  if (hosts && hosts.length) {
    unionTagkv.unshift(hosts[0]);
  }
  return unionTagkv;
}
