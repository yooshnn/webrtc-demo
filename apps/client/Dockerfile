# Development-only Dockerfile for React Router v7 + Vite
# Optimized for hot reload with Docker Compose

FROM node:20-alpine AS base

FROM base AS dependencies
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM dependencies AS dev
WORKDIR /app
# Source code mounted via docker-compose volumes
# node_modules protected by named volume
CMD ["npm", "run", "dev", "--", "--host"]