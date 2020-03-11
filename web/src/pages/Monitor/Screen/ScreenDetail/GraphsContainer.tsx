import React from 'react';
import _ from 'lodash';
import { Row } from 'antd';
import { SortableContainer } from 'react-sortable-hoc';
import RenderGraph from './RenderGraph';

function GraphsContainer(props: any) {
  return (
    <Row gutter={10}>
      {
        _.map(props.data, (item, index) => (
          <RenderGraph
            key={item.id}
            index={index}
            data={item}
            colNum={props.colNum}
            graphsInstance={props.graphsInstance}
            graphConfigForm={props.graphConfigForm}
            subclassData={props.subclassData}
            originTreeData={props.originTreeData}
            onDelChart={props.onDelChart}
            onCloneGraph={props.onCloneGraph}
          />
        ))
      }
    </Row>
  );
}

export default SortableContainer(GraphsContainer);
