import React from 'react';
import _ from 'lodash';
import { Tree } from 'antd';
import { MenuConfItem, TreeNode } from '@interface';

export function isAbsolutePath(url: string) {
  return /^https?:\/\//.test(url);
}

export function hasRealChildren(children: { visible?: boolean }[]) {
  if (_.isArray(children)) {
    return !_.every(children, item => item.visible === false);
  }
  return false;
}

export function getNsTreeVisible(activeRoutes: { nsTreeVisible?: boolean}[]) {
  return _.every(activeRoutes, route => route.nsTreeVisible === undefined ||
    route.nsTreeVisible === true);
}

export function normalizeMenuConf(children: MenuConfItem[], parentNav?: MenuConfItem) {
  const navs: MenuConfItem[] = [];

  _.each(children, (nav) => {
    if (nav.visible === undefined || nav.visible === true) {
      const navCopy = _.cloneDeep(nav);

      if (isAbsolutePath(nav.path) || _.indexOf(nav.path, '/') === 0) {
        navCopy.to = nav.path;
      } else if (parentNav) {
        if (parentNav.path) {
          const parentPath = parentNav.to ? parentNav.to : `/${parentNav.path}`;
          if (nav.path) {
            navCopy.to = `${parentPath}/${nav.path}`;
          } else {
            navCopy.to = parentPath;
          }
        } else if (nav.path) {
          navCopy.to = `/${nav.path}`;
        }
      } else if (nav.path) {
        navCopy.to = `/${nav.path}`;
      }

      if (_.isArray(nav.children) && nav.children.length && hasRealChildren(nav.children)) {
        navCopy.children = normalizeMenuConf(nav.children, navCopy);
      } else {
        delete navCopy.children;
      }

      navs.push(navCopy);
    }
  });
  return navs;
}

export function findNode(treeData: TreeNode[], node: TreeNode) {
  let findedNode: TreeNode | undefined;;
  function findNodeReal(treeData: TreeNode[], node: TreeNode) {
    _.each(treeData, (item) => {
      if (item.id === node.pid) {
        findedNode = item;
        return false;
      } else if (_.isArray(item.children)) {
        findNodeReal(item.children, node);
      }
    });
  }
  findNodeReal(treeData, node);
  return findedNode;
}

export function normalizeTreeData(data: TreeNode[]) {
  const treeData: TreeNode[] = [];
  let tag = 0;

  function fn(_cache?: TreeNode[]) {
    const cache: TreeNode[] = [];
    _.each(data, (node) => {
      node = _.cloneDeep(node);
      if (node.pid === 0) {
        if (tag === 0) {
          treeData.splice(_.sortedIndexBy(treeData, node, 'name'), 0, node);
        }
      } else {
        const findedNode = findNode(treeData, node); // find parent node
        if (!findedNode) {
          cache.push(node);
          return;
        };
        if (_.isArray(findedNode.children)) {
          if (!_.find(findedNode.children, { id: node.id })) {
            findedNode.children.splice(_.sortedIndexBy(findedNode.children, node, 'name'), 0, node);
          }
        } else {
          findedNode.children = [node];
        }
      }
    });
    tag += 1;
    if (cache.length && !_.isEqual(_cache, cache)) {
      fn(cache);
    }
  }
  fn();
  return treeData;
}

export function renderTreeNodes(nodes?: TreeNode[]) {
  return _.map(nodes, (node) => {
    if (_.isArray(node.children)) {
      return (
        <Tree.TreeNode
          title={node.name}
          key={String(node.id)}
          value={node.id}
          path={node.path}
        >
          {renderTreeNodes(node.children)}
        </Tree.TreeNode>
      );
    }
    return (
      <Tree.TreeNode
        title={node.name}
        key={String(node.id)}
        value={node.id}
        path={node.path}
        isLeaf={node.leaf === 1}
      />
    );
  });
}

export function filterTreeNodes(nodes: TreeNode[], id: number) {
  let newNodes: TreeNode[] = [];
  function makeFilter(sNodes: TreeNode[]) {
    _.each(sNodes, (node) => {
      if (node.children) {
        if (node.id === id) {
          newNodes = node.children;
        } else {
          makeFilter(node.children);
        }
      }
    });
  }
  makeFilter(nodes);
  return newNodes;
}

export function getLeafNodes(nodes: TreeNode[], nids: number[]) {
  let leafNodes: TreeNode[] = [];
  function make(cnids: number[]) {
    const n: number[] = [];
    _.each(nodes, (node: TreeNode) => {
      if (_.includes(cnids, node.pid)) {
        if (node.leaf === 1) {
          leafNodes = _.concat(leafNodes, node.id);
        } else {
          n.push(node.id);
        }
      }
    });
    if (n.length) {
      make(n);
    }
  }
  make(nids);

  if (leafNodes.length) {
    return _.uniq(leafNodes);
  }
  return nids;
}
