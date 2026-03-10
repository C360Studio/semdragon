FROM node:22-alpine

WORKDIR /app

# Copy config files first so svelte-kit sync succeeds during npm install
COPY package.json package-lock.json* tsconfig.json svelte.config.js vite.config.ts ./

RUN npm install

# Source will be mounted as volume
COPY src ./src
COPY static ./static

EXPOSE 5173

CMD ["npm", "run", "dev", "--", "--host"]
