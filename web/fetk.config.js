module.exports = {
  webpackDevConfig: 'config/webpack.dev.config.js',
  webpackBuildConfig: 'config/webpack.build.config.js',
  webpackDllConfig: 'config/webpack.dll.config.js',
  theme: 'config/theme.js',
  template: 'src/index.html',
  favicon: 'src/assets/favicon.ico',
  output: '../pub',
  eslintFix: true,
  hmr: false,
  port: 8010,
  extraBabelPlugins: [
    [
      'babel-plugin-import',
      {
        libraryName: 'antd',
        style: true,
      },
    ],
  ],
  devServer: {
    inline: true,
    proxy: {
      '/api': 'http://10.86.92.17:8058',
    },
  },
};
