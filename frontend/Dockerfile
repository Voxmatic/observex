FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY <<NGINX /etc/nginx/conf.d/default.conf
server {
    listen 3000;
    root /usr/share/nginx/html;
    index index.html;
    location / { try_files \$uri \$uri/ /index.html; }
    location /api { proxy_pass http://api-gateway:3001; }
}
NGINX
EXPOSE 3000
