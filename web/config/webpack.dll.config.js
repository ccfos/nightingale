const path = require('path');
const fs = require('fs');
const babel = require('@babel/core');

function getImportedAntCptNamesByDirPath(dirPath) {
  function union(a, b) {
    return Array.from(new Set([...a, ...b]));
  }

  function kebabCase(str) {
    return str.replace(/([a-z])([A-Z])/g, '$1-$2').replace(/\s+/g, '-').toLowerCase();
  }

  function getImportedAntCptNamesByFilePath(filePath) {
    const fileContent = fs.readFileSync(filePath, {
      encoding: 'utf8',
    });
    const importedAntCptNames = [];

    babel.transformSync(fileContent, {
      sourceType: 'module',
      presets: [
        require.resolve('@babel/preset-react'),
      ],
      plugins: [
        '@babel/plugin-proposal-class-properties',
        {
          visitor: {
            ImportDeclaration(path) {
              const { node } = path;
              if (!node) return;
              const { value } = node.source;
              node.specifiers.forEach((spec) => {
                if (value === 'antd') {
                  importedAntCptNames.push(spec.local.name);
                }
              });
            },
          },
        },
      ],
    });
    return importedAntCptNames;
  }

  function getFilesPathByDirPath(dirPath) {
    const filesPath = [];
    function make(dirPath) {
      const files = fs.readdirSync(dirPath, {
        withFileTypes: true,
      });
      files.forEach((fileDirent) => {
        if (fileDirent.isFile() && /\.(js|jsx)$/.test(fileDirent.name)) {
          const filePath = path.join(dirPath, fileDirent.name);
          filesPath.push(filePath);
        }
        if (fileDirent.isDirectory()) {
          make(path.join(dirPath, fileDirent.name));
        }
      });
    }
    make(dirPath);
    return filesPath;
  }

  const filesPath = getFilesPathByDirPath(dirPath);
  let importedAntCptNames = [];

  filesPath.forEach((filePath) => {
    const cptNames = getImportedAntCptNamesByFilePath(filePath);
    importedAntCptNames = union(importedAntCptNames, cptNames);
  });
  importedAntCptNames = importedAntCptNames.map((item) => {
    return `antd/lib/${kebabCase(item)}`;
  });

  return importedAntCptNames;
}

const importedAntCptNames = getImportedAntCptNamesByDirPath('./src/');

module.exports = function getWebpackConfig(webpackConfig) {
  webpackConfig.entry = {
    lib: [
      'react',
      'react-dom',
      '@hot-loader/react-dom',
      'react-router-dom',
      'react-sortable-hoc',
      'react-dnd',
      'react-dnd-html5-backend',
      'd3',
      'jquery',
      'lodash',
      'moment',
      'xlsx',
      'highlight.js',
      'react-highlight',
      ...importedAntCptNames,
    ],
  };
  return webpackConfig;
};
