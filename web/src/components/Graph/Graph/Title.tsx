import React, { Component } from 'react';
import _ from 'lodash';

interface Props {
  title: string,
  selectedMetric: string,
}

export default class Title extends Component<Props> {
  static defaultProps = {
    title: '',
    selectedMetric: '',
  };

  render() {
    const { title, selectedMetric } = this.props;
    const styleObj: React.CSSProperties = {
      width: '100%',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    };
    let realTitle = title;

    if (!title) {
      realTitle = selectedMetric;
    }

    return (
      <div className="graph-title">
        <div title={realTitle} style={styleObj}>
          {realTitle}
        </div>
      </div>
    );
  }
}
