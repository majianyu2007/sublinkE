<div align="center">
<img src="webs/src/assets/logo.png" width="150px" height="150px" />
</div>

<div align="center">
  <img src="https://img.shields.io/badge/Vue-5.0.8-brightgreen.svg"/>
  <img src="https://img.shields.io/badge/Go-1.24.3-green.svg"/>
  <img src="https://img.shields.io/badge/Element%20Plus-2.6.1-blue.svg"/>
  <img src="https://img.shields.io/badge/license-MIT-green.svg"/>
  <div align="center"> 中文 | <a href="README.en-US.md">English</div>


</div>

# 项目简介

`sublinkE` 是基于优秀的开源项目  [sublinkX](https://github.com/gooaclok819/sublinkX)  进行二次开发，仅在原项目基础上做了部分定制优化。建议用户优先参考和使用原项目，感谢原作者的付出与贡献。

- 前端基于 [vue3-element-admin](https://github.com/youlaitech/vue3-element-admin)；
- 后端采用 Go + Gin + Gorm；
- 默认账号：admin 密码：123456，请安装后务必自行修改；

# 修改内容


- [x] 修复部分页面BUG
- [x] 支持 Clash `dialer-proxy` 属性
- [x] 允许添加并使用 API KEY 访问 API
- [x] 导入、定时更新订阅链接中的节点
- [x] 支持AnyTLS、Socks5协议
- [x] 订阅节点排序
- [x] 支持插件扩展（实验性）
- [ ] ...

## 本 Fork 的增强

本分支在上游 `sublinkE` 基础上继续增强了自动订阅同步、输出订阅组合和 Clash/Mihomo 配置生成：

- **镜像订阅组**：自动订阅任务导入的节点由任务独立管理；更新时只替换该任务的镜像节点，保留订阅中的手动节点。
- **一组多投**：一个自动订阅任务可以同时分发到多个输出订阅；输出订阅也可以直接选择完整镜像组，无需逐个勾选节点。
- **安全同步**：下载失败、解析失败、空订阅或节点冲突时保留上一份可用数据；同步过程使用事务，避免输出处于半更新状态。
- **立即同步**：任务列表提供“立即同步”，并对同一任务加锁，避免手动同步与 Cron 任务并发覆盖。
- **HTTP/HTTPS 代理**：支持带认证、TLS、IPv4 和 IPv6 地址的 HTTP/HTTPS 代理节点，并正确生成 Clash、V2Ray 和 Surge 输出。
- **SQLite 并发改进**：启用 WAL、外键约束和写入等待，减少定时同步期间的 `database is locked`。
- **自定义 Clash/Mihomo 模板**：保留 AI、流媒体、社交、游戏平台、Microsoft、Apple、Vodafone Wi-Fi Calling、广告过滤等分流及 MRS 规则集。
- **自动节点分组**：提供香港、台湾、日本、新加坡、美国、韩国、欧洲及其他地区的自动测速组，并提供“自建优质节点”和“自建自动优选”专属组。
- **生产构建修复**：前端资源从 `webs/dist` 正确嵌入 Go 二进制，Docker 构建同步使用该目录。

已有数据库会在启动时自动迁移旧的单目标自动订阅关系。升级前仍建议备份 `db/sublink.db`。

# 项目特色

- 高自由度与安全性，支持访问订阅记录及简易配置管理；
- 支持多种客户端协议及格式，包括：
    - v2ray（base64 通用格式）
    - clash（支持 ss, ssr, trojan, vmess, vless, hy, hy2, tuic, AnyTLS, Socks5, HTTP/HTTPS）
    - surge（支持 ss, trojan, vmess, hy2, tuic, HTTP/HTTPS）
- 新增 token 授权及订阅导入功能，增强安全性和便捷性。

# 安装说明

## Docker 运行
```bash
docker run --name sublinke -p 8000:8000 \
-v $PWD/db:/app/db \
-v $PWD/template:/app/template \
-v $PWD/logs:/app/logs \
-v $PWD/plugins:/app/plugins \
-d eun1e/sublinke 
```

## 一键安装
```bash
wget https://raw.githubusercontent.com/eun1e/sublinkE/refs/heads/main/install.sh   && sh install.sh
```

> ⚠ **注意**  
> 在 **Alpine Linux** 上运行一键安装脚本时，由于 Alpine 使用 `musl` 而非 `glibc`，插件模块无法正常工作。 
> 推荐优先使用 **Docker 部署** 以获得最佳兼容性，或可选择 **Debian / Ubuntu** 等发行版。


# 插件说明

`sublinkE` 提供了灵活的插件系统，允许开发者扩展系统功能而无需修改核心代码。

## 插件开发指南

### 基本步骤

1. **创建插件文件**：参照 `plugins_examples/email_plugin.go` 编写自定义插件
2. **编译插件**：使用 `plugins_examples/build_plugin.sh email_plugin.go` 编译成 `.so` 文件
3. **部署插件**：将编译好的 `.so` 文件放入 `plugins` 目录

### 插件接口实现

所有插件必须实现 `plugins.Plugin` 接口，包含以下核心方法：

```go
// 必须实现的方法
Name() string                           // 插件名称
Version() string                        // 插件版本
Description() string                    // 插件描述
DefaultConfig() map[string]interface{}  // 默认配置
SetConfig(map[string]interface{})       // 设置配置
Init() error                            // 初始化
Close() error                           // 关闭清理

// 事件处理方法 (API 事件监听)
OnAPIEvent(ctx *gin.Context, event plugins.EventType, path string, 
           statusCode int, requestBody interface{}, 
           responseBody interface{}) error

// 声明插件关注的 API 路径和事件类型
InterestedAPIs() []string
InterestedEvents() []plugins.EventType
```

### 插件示例

系统内置以下示例插件，供开发者参考学习(版本更新可能失效，建议自己编译)：

| 插件名称 | 功能描述 | 源代码 | 编译版本 |
|---------|--------|-------|---------|
| **邮件通知插件** | 监控登录事件并发送邮件通知 | [email_plugin.go](https://github.com/eun1e/sublinkE/blob/main/plugins_examples/email_plugin.go) | [下载 .so 文件](https://raw.githubusercontent.com/eun1e/sublinkE/main/plugins_examples/email_plugin.so) |

### 插件配置与管理

可通过 Web 界面管理插件：
- 启用/禁用插件
- 配置插件参数
- 查看插件状态

## 开发自定义插件

自定义插件开发流程：

1. 创建插件 Go 文件，实现 `plugins.Plugin` 接口
2. 导出 `GetPlugin()` 函数，返回插件实例
3. 定义插件关心的 API 路径和事件类型
4. 实现事件处理逻辑
5. 使用构建脚本编译插件

```bash
# 编译插件
wget https://raw.githubusercontent.com/eun1e/sublinkE/main/plugins_examples/build_plugin.sh
chmod +x build_plugin.sh
./build_plugin.sh your_plugin.go
# 将生成的 .so 文件复制到插件目录
cp your_plugin.so ../plugins/
```

更多高级功能和详细 API 文档，请参阅代码示例。


# 项目预览

![预览1](webs/src/assets/1.png)
![预览2](webs/src/assets/2.png)
![预览3](webs/src/assets/3.png)
![预览4](webs/src/assets/4.png)
![预览5](webs/src/assets/5.png)
![预览6](webs/src/assets/6.png)
