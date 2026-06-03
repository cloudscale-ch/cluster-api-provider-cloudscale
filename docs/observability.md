# Observability

CAPCS exposes Prometheus metrics, ships Grafana dashboards, and can emit
OpenTelemetry traces. Tracing and the pprof profiler are opt-in; metrics are
always served but require opt-in wiring to be scraped.

For the developer Tilt loop, see [Development](development.md#tilt) — the
cluster-api core Tiltfile can deploy the full Prometheus / Grafana / Tempo
stack alongside CAPCS.

## Metrics

The manager exposes controller-runtime metrics on **HTTPS port 8443** at
`/metrics`, served via the `controller-manager-metrics-service` Service
(port name `https`). Authentication is via Kubernetes ServiceAccount bearer
token.

Relevant flags (defaults shown):

```
--metrics-bind-address=:8443
--metrics-secure=true
```

### Enabling scraping

The shipped `config/default/kustomization.yaml` leaves the `ServiceMonitor`
and `NetworkPolicy` commented out. To enable scraping with the
[Prometheus Operator](https://prometheus-operator.dev/):

1. Uncomment these two resources in `config/default/kustomization.yaml`:

   ```yaml
   - ../prometheus
   - ../network-policy
   ```

2. Label the namespace that runs Prometheus so the `NetworkPolicy` allows
   ingress:

   ```bash
   kubectl label namespace <prometheus-namespace> metrics=enabled
   ```

The shipped `ServiceMonitor` (`config/prometheus/monitor.yaml`) uses
`insecureSkipVerify: true` against the manager's self-signed TLS. For
production, enable the cert-manager-backed patch
`config/prometheus/monitor_tls_patch.yaml` (see comments in
`config/default/kustomization.yaml`).

## Dashboards

Three Grafana dashboards live under [`grafana/`](../grafana):

| File                                                                                    | What it shows                                                                                                              |
|-----------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| `controller-runtime-metrics.json`                                                       | Standard controller-runtime metrics: reconcile rate, queue depth, latency                                                  |
| `controller-resources-metrics.json`                                                     | Pod CPU/memory/goroutine metrics for the manager                                                                           |
| `custom-metrics/custom-metrics-dashboard.json` (and accompanying `config.yaml`)         | cloudscale.ch API call rate and error rate, by endpoint                                                                    |

The custom dashboard reads `cloudscale_requests_total`, so it works for any
workload that uses cloudscale-go-sdk v9 with the instrumented transport, not
just CAPCS.

## Tracing (opt-in)

Tracing is **off by default**. To enable it, set the following on the manager:

```
--enable-tracing=true
--tracing-sample-rate=0.1   # 0.0–1.0; default 0.1
```

Spans are exported via OTLP/gRPC (insecure). The endpoint is read from
`OTEL_EXPORTER_OTLP_ENDPOINT` (defaults to `localhost:4317`). Point it at your
collector — Tempo, Alloy, or an OpenTelemetry Collector — for example:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: http://tempo.observability.svc:4317
```

### Log/trace correlation

When tracing is enabled, every log line emitted from a reconcile carries
`trace_id` and `span_id` keys. When tracing is disabled, the keys are
omitted and logs are unchanged.

Errors logged via `logger.Error(...)` are also recorded on the active span
(`RecordError` + status `Error`), so failed reconciles show up as failed
spans in the trace UI without callers having to do anything special.

### Resource attributes

The exporter sets `service.name` (default `capcs`) and `service.version`
(the binary version), and reads:

- `OTEL_SERVICE_NAME` — overrides `service.name`. To override via
  `OTEL_RESOURCE_ATTRIBUTES=service.name=…` instead, also set
  `OTEL_SERVICE_NAME` to a non-empty value so the `capcs` default is
  skipped.
- `OTEL_RESOURCE_ATTRIBUTES` — standard OTel env var for adding resource
  attributes (e.g. `deployment.environment=prod,service.namespace=capi`).
- `POD_NAME` — populates `service.instance.id` so individual replicas are
  distinguishable in HA / leader-elected setups. The shipped
  `config/manager/manager.yaml` injects this via the Kubernetes downward
  API.

Process info (`process.pid`, `process.executable.name`,
`process.command_args`, `process.runtime.*`) is added automatically.

## Profiler (opt-in)

pprof is **off by default**. Set `--profiler-address` to bind it:

```
--profiler-address=localhost:6060
```

Bind to loopback in production and reach it via `kubectl port-forward`. Do
not expose pprof on a routable interface.
