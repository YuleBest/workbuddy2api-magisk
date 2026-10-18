#!/usr/bin/env bash
# dev.sh — 源码模式下的启停 helper（对应 docker compose 的 up/down/restart）
# 用法: ./dev.sh {start|stop|restart|status|logs|key}
set -uo pipefail
cd "$(dirname "$0")"

BIN=./wb2api
LOG=./server.log
PIDFILE=./.server.pid

alive() { [[ -f $PIDFILE ]] && kill -0 "$(cat $PIDFILE)" 2>/dev/null; }

case "${1:-status}" in
  start)
    if alive; then echo "已在运行 (PID $(cat $PIDFILE))"; exit 0; fi
    [[ -x $BIN ]] || { echo "未找到 $BIN，先执行: CGO_ENABLED=0 go build -o wb2api ./cmd/server"; exit 1; }
    nohup "$BIN" -config config.json > "$LOG" 2>&1 &
    echo $! > "$PIDFILE"
    sleep 2
    if alive; then echo "已启动 (PID $(cat $PIDFILE))"; else echo "启动失败，见 $LOG"; tail -20 "$LOG"; exit 1; fi
    ;;
  stop)
    if alive; then kill "$(cat $PIDFILE)" && echo "已停止"; else echo "未在运行"; fi
    rm -f "$PIDFILE"
    ;;
  restart)
    "$0" stop; sleep 1; "$0" start
    ;;
  status)
    if alive; then
      KEY=$(python3 -c "import json;print(json.load(open('config.json'))['api_key'])" 2>/dev/null)
      echo "运行中 (PID $(cat $PIDFILE))"
      curl -s http://127.0.0.1:7863/healthz -w "\n" 2>/dev/null
      echo "账号数: $(curl -s http://127.0.0.1:7863/status -H "Authorization: Bearer $KEY" 2>/dev/null | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("accounts",[])))' 2>/dev/null || echo '?')"
    else
      echo "未运行"
    fi
    ;;
  logs)    tail -f "$LOG" ;;
  key)     python3 -c "import json;print(json.load(open('config.json'))['api_key'])" ;;
  *)       echo "用法: $0 {start|stop|restart|status|logs|key}"; exit 1 ;;
esac
