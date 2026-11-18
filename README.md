# Dujiao-Next 二开版

本项目基于 [Dujiao-Next](https://github.com/dujiao-next/dujiao-next) 二次开发，使用 Docker Compose 和 PostgreSQL 部署。

镜像仓库：`litehub/dujiao-next-erkai`

## 技术栈

- 后端：Go、Gin、GORM
- 前端：Vue 3、TypeScript、Vite、Tailwind CSS
- 数据库：PostgreSQL 16
- 缓存和任务队列：Redis 7
- 部署：Docker Compose

## 仓库结构

```text
cmd/server/          后端服务入口
internal/            后端业务代码
frontend/user/       用户端前台
frontend/admin/      管理端后台
config.yml.example   配置示例
Dockerfile           Docker 镜像构建文件
docker-compose.yml   本地 Compose 配置
```

## Linux 部署

以下命令在 Linux 服务器执行，需要提前安装 Docker 和 Docker Compose 插件。本教程使用部署目录 `/opt/dujiao-next-erkai`。

### 1. 准备部署目录

```bash
mkdir -p /opt/dujiao-next-erkai/{config,data/db,data/uploads,data/logs,data/redis,data/postgres}
cd /opt/dujiao-next-erkai
chmod -R 0777 ./data/logs ./data/db ./data/uploads ./data/redis ./data/postgres
```

项目容器默认使用非 root 用户运行。首次部署执行上述权限设置，避免日志、上传文件和数据库目录权限不足。

### 2. 准备配置文件

```bash
curl -L https://raw.githubusercontent.com/huizhang556/dujiao-next-erkai/develop/config.yml.example \
  -o ./config/config.yml
ls -l ./config/config.yml
```

### 3. 修改 config.yml

```bash
nano ./config/config.yml
```

分别执行三次，生成三个不同的运行时密钥：

```bash
openssl rand -hex 32
```

填入：

```yaml
app:
  secret_key: 修改为第一个随机密钥

jwt:
  secret: 修改为第二个随机密钥

user_jwt:
  secret: 修改为第三个随机密钥
```

修改后台入口路径：

```yaml
web:
  admin_path: "/admin_zhanghui"
```

配置 PostgreSQL：

```yaml
database:
  driver: postgres
  dsn: host=postgres user=dujiao password=dujiao_pass dbname=dujiao_next port=5432 sslmode=disable TimeZone=Asia/Shanghai
  pool:
    max_open_conns: 50
    max_idle_conns: 10
    conn_max_lifetime_seconds: 1800
    conn_max_idle_time_seconds: 600
```

配置 Redis 和任务队列：

```yaml
redis:
  enabled: true
  host: redis
  port: 6379
  password: "db_redis_20260922"

queue:
  enabled: true
  host: redis
  port: 6379
  password: "db_redis_20260922"
  concurrency: 10
  queues:
    default: 10
    critical: 5
```

`host` 必须使用 Compose 服务名 `postgres`、`redis`，不要填写 `127.0.0.1`。密码必须和 `.env` 保持一致。

配置生产模式和首次管理员账号：

```yaml
server:
  mode: release

bootstrap:
  default_admin_username: "zhanghui"
  default_admin_password: "Zhang8020@@"
```

请修改示例账号密码。首次初始化成功后，建议删除或清空 `bootstrap` 中的账号密码配置。

保存并退出 nano：`Ctrl + O`，回车；`Ctrl + X`。

### 4. 创建 .env

```bash
cd /opt/dujiao-next-erkai
nano .env
```

填入以下内容，并修改为自己的密码：

```dotenv
TAG=latest
TZ=Asia/Shanghai
APP_PORT=8899

DJ_DEFAULT_ADMIN_USERNAME=zhanghui
DJ_DEFAULT_ADMIN_PASSWORD=Zhang8020@@

REDIS_PASSWORD=db_redis_20260922

POSTGRES_DB=dujiao_next
POSTGRES_USER=dujiao
POSTGRES_PASSWORD=dujiao_pass
```

`.env` 包含数据库密码和管理员密码，不要提交到 Git 或公开给其他人。

### 5. 创建 docker-compose.postgres.yml

```bash
nano /opt/dujiao-next-erkai/docker-compose.postgres.yml
```

填入：

```yaml
services:
  redis:
    image: redis:7-alpine
    container_name: dujiaonext-redis
    restart: unless-stopped
    command: ["redis-server", "--dir", "/data", "--appendonly", "yes", "--requirepass", "${REDIS_PASSWORD}"]
    volumes:
      - ./data/redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 10s
      timeout: 3s
      retries: 10
    networks:
      - dujiao-net

  postgres:
    image: postgres:16-alpine
    container_name: dujiaonext-postgres
    restart: unless-stopped
    environment:
      TZ: ${TZ}
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 10
    networks:
      - dujiao-net

  dujiao-next:
    image: litehub/dujiao-next-erkai:${TAG}
    container_name: dujiao-next
    restart: unless-stopped
    environment:
      TZ: ${TZ}
      DJ_DEFAULT_ADMIN_USERNAME: ${DJ_DEFAULT_ADMIN_USERNAME}
      DJ_DEFAULT_ADMIN_PASSWORD: ${DJ_DEFAULT_ADMIN_PASSWORD}
    ports:
      - "127.0.0.1:${APP_PORT}:8080"
    volumes:
      - ./config/config.yml:/app/config.yml:ro
      - ./data/uploads:/app/uploads
      - ./data/logs:/app/logs
    depends_on:
      redis:
        condition: service_healthy
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 10s
      timeout: 3s
      retries: 10
    networks:
      - dujiao-net

networks:
  dujiao-net:
    driver: bridge
```

保存并退出 nano：`Ctrl + O`，回车；`Ctrl + X`。

### 6. 配置 Nginx HTTPS 反向代理

前台、后台、API、上传文件、`sitemap.xml` 和 `robots.txt` 都由同一个应用端口提供，Nginx 只需要代理一个域名的 `/` 路径：

```nginx
server {
    listen 443 ssl http2;
    server_name shop.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;
    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:8899;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

检查并重新加载：

```bash
nginx -t
systemctl reload nginx
```

访问地址：

```text
用户前台：https://shop.example.com
管理后台：https://shop.example.com/admin_zhanghui
```

### 7. 启动和运维命令

以下命令必须在 `/opt/dujiao-next-erkai` 目录执行，并同时指定 `.env` 和 Compose 文件。

启动：

```bash
cd /opt/dujiao-next-erkai
docker compose --env-file .env -f docker-compose.postgres.yml up -d
```

查看状态：

```bash
docker compose --env-file .env -f docker-compose.postgres.yml ps
```

确认三个服务均为 `healthy`。查看应用容器日志：

```bash
docker compose --env-file .env -f docker-compose.postgres.yml logs -f dujiao-next
```

应用服务日志位于：

```text
/opt/dujiao-next-erkai/data/logs/app.log
```

停止服务：

```bash
docker compose --env-file .env -f docker-compose.postgres.yml down
```

该命令不会删除 `data/` 下的数据库、Redis、上传文件和日志。

### 8. 更新、升级和回滚

更新 `latest`：

```bash
cd /opt/dujiao-next-erkai
docker compose --env-file .env -f docker-compose.postgres.yml down
docker compose --env-file .env -f docker-compose.postgres.yml pull
docker compose --env-file .env -f docker-compose.postgres.yml up -d
```

前端随应用镜像一起更新，不需要单独操作。

升级到指定版本：

```bash
nano .env
```

将版本改为具体标签，例如：

```dotenv
TAG=1.5.4-1
```

执行：

```bash
docker compose --env-file .env -f docker-compose.postgres.yml pull
docker compose --env-file .env -f docker-compose.postgres.yml up -d
```

回滚时，将 `TAG` 改回历史版本，再执行上面的 `pull` 和 `up -d` 命令。涉及数据库结构变化的版本回滚前，请先备份 PostgreSQL 数据。

### 9. 卸载

确认已备份数据库、上传文件和日志后，停止并删除容器、网络、数据卷和镜像：

```bash
cd /opt/dujiao-next-erkai
docker compose --env-file .env -f docker-compose.postgres.yml down \
  --volumes --rmi all --remove-orphans
```

参数说明：

- `down`：停止并删除 Compose 创建的容器和网络；
- `--volumes`：删除 Compose 管理的数据卷；
- `--rmi all`：删除该 Compose 文件使用的镜像；
- `--remove-orphans`：删除同一 Compose 项目中的遗留容器。

本教程使用主机目录挂载，`--volumes` 不会删除 `/opt/dujiao-next-erkai/data/` 下的数据。确认不再需要数据后，再手动删除部署目录：

```bash
rm -rf /opt/dujiao-next-erkai
```

检查是否还有相关容器和镜像：

```bash
docker ps -a
docker images
```

确认无用后删除：

```bash
docker rm -f 容器名或容器ID
docker rmi 镜像ID
```

## 当前镜像标签

```text
litehub/dujiao-next-erkai:latest
litehub/dujiao-next-erkai:1.5.4-1
```
