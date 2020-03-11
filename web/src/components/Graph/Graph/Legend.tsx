import React, { Component } from 'react';
import { Table, Input, Button, Modal } from 'antd';
import { ColumnProps, TableRowSelection } from 'antd/es/table';
import Color from 'color';
import _ from 'lodash';
import clipboard from '@common/clipboard';
import ContextMenu from '@cpts/ContextMenu';
import { SerieInterface, PointInterface } from '../interface';

type SelectedKeys = 'normal' | string[];

interface Props {
  style: any,
  series: SerieInterface[],
  onSelectedChange: (selectedKeys: string | string[], highlightedKeysClone: string[]) => void,
}

interface State {
  searchText: string,
  filterVal: string,
  filterDropdownVisible: boolean,
  contextMenuVisiable: boolean,
  contextMenuTop: number,
  contextMenuLeft: number,
  selectedKeys: SelectedKeys, // 默认全选, 已选的 keys
  highlightedKeys: string[], // 高亮的 keys
  currentCounter: string,
}

interface LegendDataItem extends SerieInterface {
  min: number | null,
  max: number | null,
  avg: number | null,
  sum: number | null,
  last: number | null,
}

export default class Legend extends Component<Props, State> {
  static defaultProps = {
    style: {},
    series: [],
    onSelectedChange: _.noop,
  };

  state = {
    searchText: '',
    filterVal: '',
    filterDropdownVisible: false,
    contextMenuVisiable: false,
    contextMenuTop: 0,
    contextMenuLeft: 0,
    selectedKeys: 'normal', // 默认全选, 已选的 keys
    highlightedKeys: [], // 高亮的 keys
    currentCounter: '',
  } as State;

  componentWillReceiveProps(nextProps: Props) {
    const isEqualSeriesResult = isEqualSeries(this.props.series, nextProps.series);
    if (!isEqualSeriesResult) {
      this.setState({
        selectedKeys: 'normal',
        highlightedKeys: [],
      });
    }
  }

  handleInputChange = (e: any) => {
    this.setState({ searchText: e.target.value });
  }

  handleSearch = () => {
    const { searchText } = this.state;
    this.setState({
      filterDropdownVisible: false,
      filterVal: searchText,
    });
  }

  handleContextMenu = (e: any, counter: string) => {
    e.preventDefault();
    this.setState({
      currentCounter: counter,
      contextMenuVisiable: true,
      contextMenuLeft: e.clientX,
      contextMenuTop: e.clientY,
    });
  }

  handleCopyCounter = () => {
    const { currentCounter } = this.state;
    const copySucceeded = clipboard(currentCounter);
    if (!copySucceeded) {
      Modal.info({
        title: '复制失败，请手动选择复制',
        content: (
          <p>{currentCounter}</p>
        ),
      });
    }
  }

  handleClickCounter = (record: any) => {
    const { selectedKeys, highlightedKeys } = this.state;
    const highlightedKeysClone = _.clone(highlightedKeys);

    if (_.includes(highlightedKeysClone, record.id)) {
      _.remove(highlightedKeysClone, o => o === record.id);
    } else {
      highlightedKeysClone.push(record.id);
    }

    this.setState({ highlightedKeys: highlightedKeysClone }, () => {
      this.props.onSelectedChange(selectedKeys, highlightedKeysClone);
    });
  }

  filterData() {
    const { series } = this.props;
    const { filterVal } = this.state;
    const reg = new RegExp(filterVal, 'gi');
    const legendData = normalizeLegendData(series);
    return _.filter(legendData, (record) => {
      return record.tags.match(reg);
    });
  }

  render() {
    const { onSelectedChange } = this.props;
    const { searchText, selectedKeys, highlightedKeys } = this.state;
    const counterSelectedKeys = highlightedKeys;
    const data = this.filterData();
    const firstData = data[0];
    const columns: ColumnProps<LegendDataItem>[] = [
      {
        title: <span> 曲线({data.length}) </span>,
        dataIndex: 'tags',
        filterDropdown: (
          <div className="custom-filter-dropdown">
            <Input
              placeholder="请输入曲线名称"
              value={searchText}
              onChange={this.handleInputChange}
              onPressEnter={this.handleSearch}
            />
            <Button type="primary" onClick={this.handleSearch}>搜索</Button>
          </div>
        ),
        filterDropdownVisible: this.state.filterDropdownVisible,
        onFilterDropdownVisibleChange: (visible: boolean) => this.setState({ filterDropdownVisible: visible }),
        render: (text, record) => {
          const legendName = getLengendName(record);
          return (
            <span
              title={text}
              onClick={() => this.handleClickCounter(record)}
              onContextMenu={e => this.handleContextMenu(e, text)}
              style={{
                cursor: 'pointer',
                // eslint-disable-next-line no-nested-ternary
                opacity: counterSelectedKeys.length ? _.includes(counterSelectedKeys, record.id) ? 1 : 0.5 : 1,
              }}
            >
              <span style={{ color: record.color }}>● </span>
              {legendName}
            </span>
          );
        },
      }, {
        title: 'Max',
        dataIndex: 'max',
        className: 'alignRight',
        width: 100,
        render(text) {
          return <span style={{ paddingRight: 10 }}>{text}</span>;
        },
        sorter: (a, b) => Number(a.max) - Number(b.max),
      }, {
        title: 'Min',
        dataIndex: 'min',
        className: 'alignRight',
        width: 100,
        render(text) {
          return <span style={{ paddingRight: 10 }}>{text}</span>;
        },
        sorter: (a, b) => Number(a.min) - Number(b.min),
      }, {
        title: 'Avg',
        dataIndex: 'avg',
        className: 'alignRight',
        width: 100,
        render(text) {
          return <span style={{ paddingRight: 10 }}>{text !== null ? text : 'null'}</span>;
        },
        sorter: (a, b) => Number(a.avg) - Number(b.avg),
      }, {
        title: 'Sum',
        dataIndex: 'sum',
        className: 'alignRight',
        width: 100,
        render(text) {
          return <span style={{ paddingRight: 10 }}>{text !== null ? text : 'null'}</span>;
        },
        sorter: (a, b) => Number(a.sum) - Number(b.sum),
      }, {
        title: 'Last',
        dataIndex: 'last',
        className: 'alignRight',
        width: 100,
        render(text) {
          return <span style={{ paddingRight: 10 }}>{text !== null ? text : 'null'}</span>;
        },
        sorter: (a, b) => Number(a.last) - Number(b.last),
      },
    ];

    const newRowSelection: TableRowSelection<LegendDataItem> = {
      selectedRowKeys: selectedKeys === 'normal' ? _.map(data, o => o.id) : selectedKeys,
      onChange: (selectedRowKeys: string[]) => {
        this.setState({ selectedKeys: selectedRowKeys }, () => {
          onSelectedChange(selectedRowKeys, highlightedKeys);
        });
      },
    };

    if (_.get(firstData, 'isSameMetric') === false) {
      columns.unshift({
        title: '指标',
        dataIndex: 'metric',
        width: 60,
      });
    }

    return (
      <div className="graph-legend" style={{
        ...this.props.style,
        margin: '0 5px 5px 5px',
      }}>
        <Table
          rowKey={record => record.id}
          size="middle"
          rowSelection={newRowSelection}
          columns={columns}
          dataSource={data}
          pagination={false}
          scroll={{ y: 220 }}
        />
        <ContextMenu visible={this.state.contextMenuVisiable} left={this.state.contextMenuLeft} top={this.state.contextMenuTop}>
          <ul className="ant-dropdown-menu ant-dropdown-menu-vertical ant-dropdown-menu-light ant-dropdown-menu-root">
            <li className="ant-dropdown-menu-item">
              <a onClick={this.handleCopyCounter}>复制 counter</a>
            </li>
          </ul>
        </ContextMenu>
      </div>
    );
  }
}

export function normalizeLegendData(series: SerieInterface[] = []) {
  const tableData = _.map(series, (serie) => {
    const { id, metric, tags, data } = serie;
    const { last, avg, max, min, sum } = getLegendNums(data);
    return {
      id,
      metric,
      tags,
      last,
      avg,
      max,
      min,
      sum,
      color: serie.color,
    };
  });
  return _.orderBy(tableData, 'counter');
}

export function getSerieVisible(serie: SerieInterface, selectedKeys: SelectedKeys) {
  return selectedKeys === 'normal' ? true : _.includes(selectedKeys, _.get(serie, 'id'));
}

export function getSerieColor(serie: SerieInterface, highlightedKeys: string[], oldColor: string) {
  if (highlightedKeys.length && !_.includes(highlightedKeys, _.get(serie, 'id'))) {
    return Color(oldColor).lighten(0.5).desaturate(0.7).hex();
  }
  return oldColor;
}

export function getSerieIndex(serie: SerieInterface, highlightedKeys: string[], seriesLength: number, serieIndex: number) {
  return _.includes(highlightedKeys, _.get(serie, 'id')) ? seriesLength + serieIndex : serieIndex;
}

function getLegendNums(points: PointInterface[]) {
  let last: number | null = null;
  let avg: number | null = null;
  let max: number | null = null;
  let min: number | null = null;
  let sum: number | null = null;
  let len = 0;

  if (!_.isArray(points)) {
    return { last, avg, max, min, sum };
  }

  _.forEach(points, (point) => {
    const x = _.get(point, '[0]');
    const y = _.get(point, '[1]');
    if (typeof x === 'number' && typeof y === 'number') {
      if (sum === null) sum = 0;
      sum += y;

      if (max === null || max < y) {
        max = y;
      }

      if (min === null || min > y) {
        min = y;
      }

      last = y;
      len++;
    }
  });

  if (_.isNumber(sum)) {
    avg = sum / len;
  }

  if (typeof last === 'number') {
    last = Number(Number(last).toFixed(3));
  }

  if (typeof avg === 'number') {
    avg = Number(Number(avg).toFixed(3));
  }

  if (typeof max === 'number') {
    max = Number(Number(max).toFixed(3));
  }

  if (typeof min === 'number') {
    min = Number(Number(min).toFixed(3));
  }

  if (typeof sum === 'number') {
    sum = Number(Number(sum).toFixed(3));
  }
  return { last, avg, max, min, sum };
}

function getLengendName(serie: SerieInterface) {
  const { tags } = serie;
  let lname = tags;
  // shorten name
  if (lname.length > 80) {
    const leftStr = lname.substr(0, 40);
    const rightStr = lname.substr(-40);
    lname = `${leftStr}......${rightStr}`;
  }
  return lname;
}

function isEqualSeries(series: SerieInterface[], nextSeries: SerieInterface[]) {
  const pureSeries = _.map(series, (serie) => {
    return serie.id;
  });
  const pureNextSeries = _.map(nextSeries, (serie) => {
    return serie.id;
  });
  return _.isEqual(pureSeries, pureNextSeries);
}
