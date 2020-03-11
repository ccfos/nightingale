import _ from 'lodash';
import { GraphDataInterface } from '../interface';

export default function getYAxis(yAxis: any, graphConfig: GraphDataInterface) {
  const { threshold, yAxisMin, yAxisMax } = graphConfig;
  const newYAxis = _.clone(yAxis);

  if (threshold !== undefined && threshold !== null) {
    newYAxis.plotLines = [{
      value: threshold,
      color: 'red',
    }];
  } else {
    delete newYAxis.plotLines;
  }

  if (yAxisMin !== undefined && yAxisMin !== null && yAxisMax !== undefined && yAxisMax !== null) {
    newYAxis.min = yAxisMin;
    newYAxis.max = yAxisMax;
  } else {
    delete newYAxis.min;
    delete newYAxis.max;
  }
  return newYAxis;
}
