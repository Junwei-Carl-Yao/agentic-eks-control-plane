FROM node:20-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# nginx-unprivileged listens on 8080 by default and runs as UID 101 (nginx),
# which lets the Pod set runAsNonRoot + readOnlyRootFilesystem without an
# init container chowning anything. Cache + pid dirs come from emptyDir
# volumes mounted by the chart.
FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
