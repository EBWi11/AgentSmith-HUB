const config = {
  // API Configuration
  // 自动检测环境：开发环境使用localhost，生产环境使用当前域名
  apiBaseUrl: process.env.NODE_ENV === 'development' 
    ? 'http://localhost:8080'
    : `${window.location.protocol}//${window.location.hostname}:8080`,
  apiTimeout: 30000, // 30 seconds
};

export default config; 