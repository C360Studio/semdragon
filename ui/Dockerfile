FROM node:22-alpine

WORKDIR /app

# Install dependencies first for better caching
COPY package.json package-lock.json* ./
RUN npm install

# Copy config files
COPY tsconfig.json svelte.config.js vite.config.ts vitest.setup.ts ./

# Source will be mounted as volume
COPY src ./src
COPY static ./static

EXPOSE 5173

CMD ["npm", "run", "dev", "--", "--host"]
