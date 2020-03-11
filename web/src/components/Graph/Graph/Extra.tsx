import React, { Component } from 'react';
import { Icon, Dropdown, Menu } from 'antd';
import Info from './Info';
import { GraphDataInterface, CounterInterface } from '../interface';

interface Props {
  graphConfig: GraphDataInterface,
  counterList: CounterInterface[],
  moreList: React.ReactNode,
  onOpenGraphConfig: (graphConfig: GraphDataInterface) => void,
}

export default class Extra extends Component<Props> {
  static defaultProps = {
    moreList: null,
    counterList: [],
    onOpenGraphConfig: () => {},
  };

  onOpenGraphConfig = () => {
    this.props.onOpenGraphConfig(this.props.graphConfig);
  }

  render() {
    return (
      <div style={{ display: 'inline-block' }}>
        <span className="graph-extra-item">
          <Info
            graphConfig={this.props.graphConfig}
            counterList={this.props.counterList}
          >
            <Icon type="info-circle-o" />
          </Info>
        </span>
        <span className="graph-extra-item">
          <Icon onClick={this.onOpenGraphConfig} type="setting" />
        </span>
        <span className="graph-extra-item">
          <Dropdown trigger={['click']} overlay={
            <Menu>
              {this.props.moreList}
            </Menu>
          }>
            <span>
              <Icon type="bars" />
            </span>
          </Dropdown>
        </span>
      </div>
    );
  }
}
