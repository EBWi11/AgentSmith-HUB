const config = {
  // API Configuration
  apiBaseUrl: process.env.NODE_ENV === 'development' 
    ? 'http://localhost:8080'
    : `${window.location.protocol}//${window.location.hostname}:8080`,
  apiTimeout: 30000, // 30 seconds
};

export default config; 