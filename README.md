# Dujiao-Next 二开版

本项目基于 [Dujiao-Next](https://github.com/dujiao-next/dujiao-next) 二次开发，使用 Docker Compose、PostgreSQL 和 Redis 部署。

镜像仓库：litehub/dujiao-next-erkai

## 技术栈

- 后端：Go、Gin、GORM
- 前端：Vue 3、TypeScript、Vite、Tailwind CSS
- 数据库：PostgreSQL 16
- 缓存和任务队列：Redis 7
- 部署：Docker Compose

## 仓库结构

~~~text
cmd/server/          后端服务入口
internal/            后端业务代码
frontend/user/       用户端前台
frontend/admin/      管理端后台
config.yml.example   配置示例
Dockerfile           Docker 镜像构建文件
docker-compose.yml   本地 Compose 配置
~~~

## Linux 部署

以下操作均在 Linux 服务器执行。请提前安装 Docker Engine、Docker Compose 插件和 Nginx，并确保域名已经解析到服务器。

本教程使用部署目录 /opt/dujiao-next-erkai，最终运行三个容器：dujiao-next、dujiaonext-redis、dujiaonext-postgres。

### 1. 创建部署目录

~~~bash
# 创建配置、上传文件、日志、Redis 和 PostgreSQL 数据目录
mkdir -p /opt/dujiao-next-erkai/config /opt/dujiao-next-erkai/data/{uploads,logs,redis,postgres} \
  && cd /opt/dujiao-next-erkai

# 允许容器写入挂载目录；首次部署建议执行
chmod -R 0777 ./data/uploads ./data/logs ./data/redis ./data/postgres
~~~

不要删除 data/ 目录。这里保存 PostgreSQL 数据、Redis 数据、上传文件和应用日志。

### 2. 下载配置模板

~~~bash
# 从本项目 develop 分支下载配置模板
curl -fL https://raw.githubusercontent.com/huizhang556/dujiao-next-erkai/develop/config.yml.example \
  -o ./config/config.yml \
  && test -s ./config/config.yml \
  && echo "配置文件下载成功：./config/config.yml"
~~~

检查文件：

~~~bash
ls -l ./config/config.yml
~~~

### 3. 修改 config.yml

~~~bash
nano ./config/config.yml
~~~

执行三次下面的命令，每次生成一个不同的密钥：

~~~bash
openssl rand -hex 32
~~~

将结果分别填入：

~~~yaml
app:
  secret_key: 第一个随机密钥

jwt:
  secret: 第二个随机密钥

user_jwt:
  secret: 第三个随机密钥
~~~

修改后台入口路径，不能使用 /、/api、/uploads 或 /health：

~~~yaml
web:
  admin_path: "/admin"
~~~

配置 PostgreSQL，host 必须使用 Compose 服务名 postgres：

~~~yaml
database:
  driver: postgres
  dsn: host=postgres user=dujiao password=dujiao_pass dbname=dujiao_next port=5432 sslmode=disable TimeZone=Asia/Shanghai
  pool:
    max_open_conns: 50
    max_idle_conns: 10
    conn_max_lifetime_seconds: 1800
    conn_max_idle_time_seconds: 600
~~~

配置 Redis 和任务队列，host 必须使用 Compose 服务名 redis：

~~~yaml
redis:
  enabled: true
  host: redis
  port: 6379
  # 应用连接 Redis 使用的密码，必须与 .env 中的 REDIS_PASSWORD 完全一致
  password: "db_redis_passwd"

queue:
  enabled: true
  host: redis
  port: 6379
  # 应用连接 Redis 任务队列使用的密码，也必须与 .env 中的 REDIS_PASSWORD 完全一致
  password: "db_redis_passwd"
  db: 1
  concurrency: 10
  queues:
    default: 10
    critical: 5
~~~

redis.password 和 queue.password 必须使用完全相同的密码，并且必须与 .env 中的 REDIS_PASSWORD 一致。这里没有自动覆盖关系：.env 中的 REDIS_PASSWORD 只会配置 Redis 容器，config.yml 中的两个 password 才是应用连接 Redis 时使用的密码，三处必须手动保持一致。

设置生产模式和首次管理员账号密码：

项目启用强密码策略。管理员密码至少需要包含大写字母、小写字母和数字，建议同时包含特殊字符，长度不少于 8 位。下面的 Admin123! 只是示例，生产环境请替换为你自己的强密码；不能使用 admin123，否则应用会启动失败。

~~~yaml
server:
  mode: release

bootstrap:
  # 只有 .env 中未提供 DJ_DEFAULT_ADMIN_USERNAME/PASSWORD 时，才使用这里的值
  default_admin_username: "admin"
  default_admin_password: "Admin123!"
~~~

保存并退出 nano：按 Ctrl + O，回车确认；再按 Ctrl + X。

### 4. 创建 .env

~~~bash
# 创建环境变量文件
nano /opt/dujiao-next-erkai/.env
~~~

填入以下内容，并把管理员密码改成自己的强密码。该密码必须符合项目密码策略：至少包含大写字母、小写字母和数字，建议同时包含特殊字符，长度不少于 8 位。

~~~dotenv
# 使用 latest 时，每次更新都会获取 Docker Hub 最新镜像
TAG=latest
TZ=Asia/Shanghai

# 应用只绑定到本机，由 Nginx 提供 HTTPS
APP_PORT=8899

# 传给应用容器，优先级高于 config.yml 中 bootstrap 的同名配置；仅首次初始化管理员时使用
DJ_DEFAULT_ADMIN_USERNAME=admin
DJ_DEFAULT_ADMIN_PASSWORD=Admin123!

# 传给 Redis 容器；不会自动修改 config.yml，必须与其中 redis.password 和 queue.password 手动保持一致
REDIS_PASSWORD=db_redis_passwd

# 必须与 config/config.yml 中 PostgreSQL DSN 的参数一致
POSTGRES_DB=dujiao_next
POSTGRES_USER=dujiao
POSTGRES_PASSWORD=dujiao_pass
~~~

保存并退出 nano：按 Ctrl + O，回车确认；再按 Ctrl + X。

.env 包含数据库密码和管理员密码，不要提交到 Git，也不要公开发布。

### 5. 创建 PostgreSQL Compose 文件

~~~bash
nano /opt/dujiao-next-erkai/docker-compose.postgres.yml
~~~

完整填入：

~~~yaml
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
~~~

保存并退出 nano：按 Ctrl + O，回车确认；再按 Ctrl + X。

检查 Compose 文件格式和变量替换：

~~~bash
# 只检查配置，不会启动容器
docker compose --env-file .env -f docker-compose.postgres.yml config
~~~

正常情况下会输出展开变量后的 Compose 配置。最前面的 `name` 会根据部署目录名称变化，不需要固定为某个值；后面的内容中应能看到三个服务：

~~~text
name: dujiao-next-erkai
services:
  dujiao-next:
    image: litehub/dujiao-next-erkai:latest
  postgres:
    image: postgres:16-alpine
  redis:
    image: redis:7-alpine
...
~~~

实际输出会比上面更长，还会显示端口、挂载目录、健康检查和网络配置。只要命令正常结束、没有出现 error，并且变量已经被替换，就可以继续。这个命令只检查 Compose 文件，不会拉取镜像、创建容器或启动项目。

如果出现变量未设置、YAML 格式错误或文件不存在等错误，先修正错误，不要继续启动。

### 6. 选择访问方式

请根据自己的情况选择一种方式。项目必须先启动成功，才能访问前台或后台。

#### 方式一：有域名，使用 Nginx HTTPS

如果服务器已经有域名和 HTTPS 证书，可以使用 Nginx 反向代理。Nginx 可以手动安装配置，也可以由宝塔等服务器面板管理。使用面板时，请在面板的网站设置、反向代理和 SSL 设置中填写同样的代理参数，不需要再次执行下面的 systemctl 命令。

将 shop.example.com 换成你的域名，将证书路径换成实际路径。下面的配置适用于手动管理 Nginx 的情况：

~~~nginx
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
~~~

~~~bash
nginx -t && systemctl reload nginx
~~~

如果使用宝塔或其他面板，请使用面板完成以下配置：

- 网站域名：你的实际域名
- 反向代理目标：`http://127.0.0.1:8899`
- WebSocket：按面板选项开启
- SSL：在面板中申请或配置证书，并开启 HTTPS

保持 Compose 中的 `127.0.0.1:${APP_PORT}:8080` 不变，这样应用端口不会直接暴露到公网。

#### 方式二：没有域名，使用服务器 IP + 端口

如果暂时没有域名和 HTTPS，可以直接使用服务器 IP 访问。先编辑 `docker-compose.postgres.yml`，将：

~~~yaml
- "127.0.0.1:${APP_PORT}:8080"
~~~

改为：

~~~yaml
- "${APP_PORT}:8080"
~~~

然后在服务器防火墙和云服务器安全组中放行 `APP_PORT`，再启动项目。假设 `.env` 中 `APP_PORT=8899`，访问地址为：

~~~text
用户前台：http://服务器IP:8899
管理后台：http://服务器IP:8899/admin
~~~

这种方式是 HTTP，不是 HTTPS，只适合临时测试或内网使用。正式部署建议使用域名、Nginx 和 HTTPS。

### 7. 启动、查看和停止

先启动项目：

~~~bash
cd /opt/dujiao-next-erkai

# 首次启动会拉取 PostgreSQL、Redis 和项目镜像
docker compose --env-file .env -f docker-compose.postgres.yml up -d

# 查看三个容器状态；等待三个服务变为 healthy
docker compose --env-file .env -f docker-compose.postgres.yml ps
~~~

确认三个服务都处于 `Up`，并且 PostgreSQL、Redis、应用容器的健康状态为 `healthy` 后，再访问网站。

访问地址：

- 使用域名和 HTTPS：`https://你的域名`；后台为 `https://你的域名/admin`
- 使用 IP 和端口：`http://服务器IP:8899`；后台为 `http://服务器IP:8899/admin`

查看日志：

~~~bash
# 持续查看容器日志，按 Ctrl + C 退出查看，不会停止容器
docker compose --env-file .env -f docker-compose.postgres.yml logs -f dujiao-next

# 查看应用文件日志
tail -f /opt/dujiao-next-erkai/data/logs/app.log
~~~

停止本项目容器：

~~~bash
# 停止并删除容器和 Compose 网络，保留 data/ 下的所有数据
docker compose --env-file .env -f docker-compose.postgres.yml down
~~~

### 8. 更新、升级和回滚

使用 latest 更新：

~~~bash
cd /opt/dujiao-next-erkai

# 停止旧容器，拉取 latest，使用新镜像重新创建并启动
docker compose --env-file .env -f docker-compose.postgres.yml down \
  && docker compose --env-file .env -f docker-compose.postgres.yml pull \
  && docker compose --env-file .env -f docker-compose.postgres.yml up -d
~~~

升级到指定版本：

~~~bash
# 修改 TAG，例如 1.5.4-2
nano .env

docker compose --env-file .env -f docker-compose.postgres.yml pull \
  && docker compose --env-file .env -f docker-compose.postgres.yml up -d
~~~

回滚时，将 .env 中的 TAG 改回历史版本，再执行同样的 pull 和 up -d 命令。升级或回滚前请先备份 PostgreSQL 数据。

### 9. 卸载

确认已经备份数据库、上传文件和日志后，执行：

~~~bash
cd /opt/dujiao-next-erkai

# 删除容器、网络和 Compose 使用的镜像；不会删除 data/ 下的主机目录数据
docker compose --env-file .env -f docker-compose.postgres.yml down \
  --rmi all --remove-orphans
~~~

确认不再需要数据后，才删除部署目录：

~~~bash
# 此命令会永久删除配置、数据库、上传文件和日志
rm -rf /opt/dujiao-next-erkai
~~~
