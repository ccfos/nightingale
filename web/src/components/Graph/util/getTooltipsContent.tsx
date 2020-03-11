/* eslint-disable no-use-before-define */
import _ from 'lodash';
import moment from 'moment';
import numeral from 'numeral';
import { PointInterface } from '../interface';

interface ActiveTooltipData {
  chartWidth: number,
  isComparison: boolean,
  points: PointInterface[],
  originalPoints: PointInterface[],
  sharedSortDirection: 'desc' | 'asc',
  comparison: string[],
  relativeTimeComparison: boolean,
  timezoneOffset: string | number,
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
    tooltipContent += singlePoint(point);
  });

  return `<div style="table-layout: fixed;max-width: ${chartWidth}px;word-wrap: break-word;white-space: normal;">${tooltipContent}</div>`;
}

function singlePoint(pointData = {} as PointInterface) {
  const { color, filledNull, serieOptions = {} } = pointData;
  const { tags } = serieOptions as any;
  const value = numeral(pointData.value).format('0,0[.]000');

  return (
    `<span style="color:${color}">● </span>
    ${_.escape(tags)}：<strong>${value}${filledNull ? '(空值填补,仅限看图使用)' : ''}</strong><br />`
  );
}

function getHeaderStr(activeTooltipData: ActiveTooltipData) {
  const { points } = activeTooltipData;
  const dateStr = moment(points[0].timestamp).format(fmt);
  const headerStr = `<span style="color: #666">${dateStr}</span><br/>`;
  return headerStr;
}
