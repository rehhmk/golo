// Backend API base URL. Set via VITE_API_BASE_URL at build time (see
// .env.production) for deployed builds; falls back to localhost for local
// dev against `make run`.
export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';
