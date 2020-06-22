/* eslint-disable no-use-before-define */
import _ from 'lodash';
import moment from 'moment';
import numeral from 'numeral';
import { PointInterface } from '../interface';

interface ActiveTooltipData {
  chartWidth: number,
  isComparison: boolean,
  points: PointInterface[],
  originalPoints?: PointInterface[],
  sharedSortDirection?: 'desc' | 'asc',
  comparison: string[],
  relativeTimeComparison?: boolean,
  timezoneOffset?: string | number,
}

const fmt = 'YYYY-MM-DD HH:mm:ss';

export default function getTooltipsContent(activeTooltipData: ActiveTooltipData) {
  const { chartWidth, isComparison, points } = activeTooltipData;
  const sortedPoints = _.orderBy(points, (point) => {
    const { series } = point;
    if (isComparison) {
      const { comparison } = series ? series.userOptions : { comparison: 0 };
      return Number(comparison) || 0;
    }
    return _.get(series, 'userOptions.tags');
  });
  let tooltipContent = '';

  tooltipContent += getHeaderStr(activeTooltipData);

  _.each(sortedPoints, (point) => {
    tooltipContent += singlePoint(point, activeTooltipData);
  });

  return `<div style="table-layout: fixed;max-width: ${chartWidth}px;word-wrap: break-word;white-space: normal;">${tooltipContent}</div>`;
}

function singlePoint(pointData: any = {}, activeTooltipData: any) {
  const { color, filledNull, serieOptions = {}, timestamp } = pointData;
  const { comparison: comparisons, isComparison } = activeTooltipData;
  const { tags } = serieOptions;
  const value = numeral(pointData.value).format('0,0[.]000');
  let name = tags;

  name = _.chain(name).replace('<', '&lt;').replace('>', '&gt;').value();

  // 对比情况下 name 特殊处理
  if (isComparison) {
    const mDate = serieOptions.comparison && typeof serieOptions.comparison === 'number' ? moment(timestamp).subtract(serieOptions.comparison, 'seconds') : moment(timestamp);
    const isAllDayLevelComparison = _.every(comparisons, (o) => {
      return _.isInteger(Number(o) / 86400000);
    });

    if (isAllDayLevelComparison) {
      const dateStr = mDate.format('YYYY-MM-DD');
      name = `${dateStr}`;
    } else {
      const dateStr = mDate.format(fmt);
      name = `${dateStr} ${name}`;
    }
  }

  return (
    `<span style="color:${color}">● </span>
    ${name}：<strong>${value}${filledNull ? '(空值填补,仅限看图使用)' : ''}</strong><br />`
  );
}

function getHeaderStr(activeTooltipData: ActiveTooltipData) {
  const { points } = activeTooltipData;
  const dateStr = moment(points[0].timestamp).format(fmt);
  const headerStr = `<span style="color: #666">${dateStr}</span><br/>`;
  return headerStr;
}
