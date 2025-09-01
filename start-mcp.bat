@echo off

SET GRAFANA_API_KEY=glsa_UBRTEOdwKBbNNEXJNUMSyEe1pasrMSJT_fea7d49d

SET GRAFANA_URL=https://monitoring-cluster-brt-nprd.bankingcloud.moodysanalytics.net/grafana

call mcp-grafana.exe -t sse -config config-examples\test-config.yaml -debug