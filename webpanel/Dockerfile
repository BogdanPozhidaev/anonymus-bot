FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginx:1.25-alpine
COPY nginx.conf /etc/nginx/nginx.conf
COPY --chmod=755 docker-panel-config.sh /panel-entrypoint.sh
COPY --from=build /app/dist/ /usr/share/nginx/html/
EXPOSE 8080
USER 10001:10001
ENTRYPOINT ["/panel-entrypoint.sh"]
CMD ["nginx", "-g", "daemon off;"]
