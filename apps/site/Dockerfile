FROM node:22-bookworm-slim AS build

WORKDIR /app

COPY apps/site/package.json apps/site/package-lock.json ./
RUN npm ci

COPY apps/site ./
RUN npm run generate

FROM nginxinc/nginx-unprivileged:1.27-alpine

COPY --from=build /app/.output/public /usr/share/nginx/html

EXPOSE 8080
