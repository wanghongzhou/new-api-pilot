# Prometheus 配置

`prometheus.example.yaml` 是最小抓取示例。部署时把应用地址替换为受控网络中的实际地址，并确保 Prometheus 来源 IP 命中 `METRICS_ALLOWED_CIDRS`。

加载时将三份文件分别挂载到：

- `/etc/prometheus/prometheus.yml`
- `/etc/prometheus/recording-rules.yaml`
- `/etc/prometheus/alert-rules.yaml`

发布前执行：

```bash
make check-prometheus
```

`receiver_scope="infrastructure"` 的告警必须路由到独立于应用钉钉 Worker 的 Alertmanager receiver。备份指标由 node-exporter textfile collector 或等价基础设施采集器提供；数据库与应用时钟偏移由应用指标采样器直接提供。

备份任务设置 `PROMETHEUS_TEXTFILE_DIR` 后，`scripts/backup.sh` 会通过同目录临时文件加原子重命名维护 `new_api_pilot_backup.prom`。使用 `node-exporter.compose.example.yaml` 启用 textfile collector，并让 `prometheus.example.yaml` 的 `new-api-pilot-infrastructure` job 抓取它。目录必须是绝对路径、真实目录，且备份进程可写、node-exporter 可读。

`new_api_pilot_clock_offset_seconds` 由应用每分钟比较 MySQL `UNIX_TIMESTAMP()` 与应用时钟后直接暴露，不依赖另一个未配置的 textfile 生产者。
