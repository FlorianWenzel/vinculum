FROM node:22-bookworm-slim AS build

WORKDIR /app

COPY apps/hive-ui/package.json apps/hive-ui/package-lock.json ./
RUN npm ci

COPY apps/hive-ui ./
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.27-alpine

COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080
