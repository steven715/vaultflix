# Build stage: compile the React SPA to static files.
# Build context is the repo root (see docker-compose.yml) so this stage can
# reach ./web; the root .dockerignore excludes web/node_modules and web/dist.
FROM node:20-alpine AS build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Serve stage: bake the static bundle into the nginx image. The frontend is now
# an immutable artifact (no web_dist shared volume), symmetric with the API image.
FROM nginx:alpine
COPY nginx/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /web/dist /usr/share/nginx/html
EXPOSE 80
