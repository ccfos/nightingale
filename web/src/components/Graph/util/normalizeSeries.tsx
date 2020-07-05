import _ from 'lodash';
import { hexPalette } from '../config';
import { SerieInterface, GraphDataInterface } from '../interface';

export default function normalizeSeries(data: any[], graphConfig: GraphDataInterface): SerieInterface[] {
  const { comparison } = graphConfig;
  const isComparison = !!_.get(comparison, 'length', 0);
  const series = [] as SerieInterface[];
  _.each(_.sortBy(data, ['counter', 'endpoint']), (o, i) => {
    const { endpoint, comparison } = o;
    const color = getSerieColor(o, i, isComparison);
    const separatorIdx = o.counter.indexOf('/');

    let counter = endpoint ? '' : o.counter;
    if (separatorIdx > -1) {
      counter = `,${o.counter.substring(o.counter.indexOf('/') + 1)}`;
    }

    const id = `${endpoint}${counter}-${comparison}`;
    const name = `${endpoint}${counter}`;
    const serie = {
      id,
      name: name,
      tags: name,
      data: o.values,
      lineWidth: 2,
      color,
      oldColor: color,
      comparison,
    } as SerieInterface;
    series.push(serie);
  });

  return series;
}

function getSerieColor(serie: SerieInterface, serieIndex: number, isComparison: boolean): string {
  const { comparison } = serie;
  let color;
  // 同环比固定曲线颜色
  if (isComparison && !comparison) {
    // 今天绿色
    color = 'rgb(67, 150, 30)';
  } else if (comparison === 86400) {
    // 昨天蓝色
    color = 'rgb(98, 127, 202)';
  } else if (comparison === 604800) {
    // 上周红色
    color = 'rgb(238, 92, 90)';
  } else {
    const colorIndex = serieIndex % hexPalette.length;
    color = hexPalette[colorIndex];
  }

  return color;
}
