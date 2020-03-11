var webpackConfigResolveAlias = require('./webpackConfigResolveAlias');

module.exports = function(webpackConfig, webpack) {
  webpackConfig.resolve.alias = webpackConfigResolveAlias;
  return webpackConfig;
}
