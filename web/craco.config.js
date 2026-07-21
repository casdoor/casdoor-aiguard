const CracoLessPlugin = require("craco-less");
const webpack = require("webpack");

module.exports = {
  webpack: {
    configure: (webpackConfig) => {
      // node-casbin, which the Policy Hub runs in the browser, brings two Node
      // assumptions with it: its file adapter lazily imports "fs", which we
      // never use here, and its CSV parser expects a global Buffer.
      webpackConfig.resolve.fallback = {...webpackConfig.resolve.fallback, fs: false, buffer: require.resolve("buffer/")};
      webpackConfig.plugins.push(new webpack.ProvidePlugin({Buffer: ["buffer", "Buffer"]}));
      return webpackConfig;
    },
  },
  devServer: {
    proxy: {
      "/api": {
        target: "http://localhost:9000",
        changeOrigin: true,
      },
    },
  },
  plugins: [
    {
      plugin: CracoLessPlugin,
      options: {
        lessLoaderOptions: {
          lessOptions: {
            javascriptEnabled: true,
          },
        },
      },
    },
  ],
};
