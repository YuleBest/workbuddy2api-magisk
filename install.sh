#!/usr/bin/env bash
set -Eeuo pipefail

REPO="${WB2API_REPO:-YuleBest/workbuddy2api}"
VERSION="${WB2API_VERSION:-v1.0.1}"
RELEASE_BASE_URL="${WB2API_RELEASE_BASE_URL:-https://github.com/${REPO}/releases/download/${VERSION}}"
DATA_HOME="${XDG_DATA_HOME:-${HOME}/.local/share}"
BIN_HOME="${WB2API_BIN_DIR:-${HOME}/.local/bin}"
INSTALL_DIR="${WB2API_HOME:-${DATA_HOME}/wb2api}"
WORK_DIR=""

say() {
	printf '%s\n' "$*"
}

die() {
	printf '错误: %s\n' "$*" >&2
	exit 1
}

cleanup() {
	if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
		rm -rf -- "${WORK_DIR}"
	fi
}

need_command() {
	command -v "$1" >/dev/null 2>&1 || die "缺少命令: $1"
}

trap cleanup EXIT
trap 'die "安装在第 ${LINENO} 行失败"' ERR

[[ "$(uname -s)" == "Linux" ]] || die "当前安装器只支持 Linux"

case "$(uname -m)" in
x86_64 | amd64)
	GOARCH="amd64"
	;;
aarch64 | arm64)
	GOARCH="arm64"
	;;
*)
	die "不支持的 CPU 架构: $(uname -m)"
	;;
esac

for command_name in curl tar awk sha256sum install od tr; do
	need_command "${command_name}"
done

[[ -n "${HOME:-}" ]] || die "HOME 未设置"

ASSET="workbuddy2api-${VERSION}-linux-${GOARCH}.tar.gz"
CHECKSUM_FILE="SHA256SUMS"

say "WorkBuddy2API 一键安装"
say "版本: ${VERSION}"
say "平台: linux/${GOARCH}"
say "目录: ${INSTALL_DIR}"

mkdir -p "${DATA_HOME}"
WORK_DIR="$(mktemp -d "${DATA_HOME}/.wb2api-install.XXXXXX")"
mkdir -p "${WORK_DIR}/package"

say "下载 ${ASSET}"
curl -fL --retry 3 --retry-delay 1 \
	-o "${WORK_DIR}/${CHECKSUM_FILE}" \
	"${RELEASE_BASE_URL}/${CHECKSUM_FILE}"
curl -fL --retry 3 --retry-delay 1 \
	-o "${WORK_DIR}/${ASSET}" \
	"${RELEASE_BASE_URL}/${ASSET}"

EXPECTED_SHA="$(awk -v name="${ASSET}" '$2 == name { print $1; exit }' "${WORK_DIR}/${CHECKSUM_FILE}")"
[[ "${EXPECTED_SHA}" =~ ^[0-9a-fA-F]{64}$ ]] || die "SHA256SUMS 中找不到 ${ASSET}"
ACTUAL_SHA="$(sha256sum "${WORK_DIR}/${ASSET}" | awk '{ print $1 }')"
[[ "${ACTUAL_SHA}" == "${EXPECTED_SHA}" ]] || die "SHA-256 校验失败"
say "SHA-256 校验通过"

tar -xzf "${WORK_DIR}/${ASSET}" -C "${WORK_DIR}/package"
for required_file in wb2api wbapi login config.example.json; do
	[[ -f "${WORK_DIR}/package/${required_file}" ]] || die "发布包缺少 ${required_file}"
done

mkdir -p "${INSTALL_DIR}" "${BIN_HOME}"

for package_file in "${WORK_DIR}/package"/*; do
	[[ -f "${package_file}" ]] || continue
	file_name="$(basename "${package_file}")"
	mode="0644"
	if [[ -x "${package_file}" ]]; then
		mode="0755"
	fi
	install -m "${mode}" "${package_file}" "${INSTALL_DIR}/.${file_name}.installing"
	mv -f -- "${INSTALL_DIR}/.${file_name}.installing" "${INSTALL_DIR}/${file_name}"
done

NEW_API_KEY=""
if [[ ! -f "${INSTALL_DIR}/config.json" ]]; then
	NEW_API_KEY="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
	(
		umask 077
		cat >"${INSTALL_DIR}/config.json" <<EOF_CONFIG
{
  "listen": "127.0.0.1:7863",
  "api_key": "${NEW_API_KEY}",
  "admin": {
    "enabled": true
  }
}
EOF_CONFIG
	)
	chmod 600 "${INSTALL_DIR}/config.json"
fi

mkdir -p "${INSTALL_DIR}/auths" "${INSTALL_DIR}/data"

link_command() {
	local name="$1"
	local target="$2"
	local destination="${BIN_HOME}/${name}"
	if [[ -e "${destination}" && ! -L "${destination}" ]]; then
		say "跳过 ${destination}：文件已存在"
		return
	fi
	ln -sfn "${target}" "${destination}"
}

link_command "wb2api" "${INSTALL_DIR}/wb2api"
link_command "wbapi" "${INSTALL_DIR}/wbapi"
link_command "wb2api-login" "${INSTALL_DIR}/login"

PANEL_URL="$("${INSTALL_DIR}/wbapi" panel 2>/dev/null | awk -F': ' '/^管理后台: / { print $2; exit }')"
if [[ -z "${PANEL_URL}" ]]; then
	PANEL_URL="http://127.0.0.1:7863/admin/"
fi

say ""
say "安装完成"
say "运行目录: ${INSTALL_DIR}"
say "管理面板: ${PANEL_URL}"
if [[ -n "${NEW_API_KEY}" ]]; then
	say "调用密钥: ${NEW_API_KEY}"
	say "请妥善保存该密钥，后续不会再次显示"
fi
say ""
say "添加账号:"
say "  cd ${INSTALL_DIR}"
say "  ./login"
say ""
say "启动网关:"
say "  cd ${INSTALL_DIR}"
say "  ./wb2api -config config.json"
say ""
say "未创建 systemd 服务、计划任务或 cloudflared 配置"

case ":${PATH}:" in
*":${BIN_HOME}:"*) ;;
*)
	say ""
	say "提示: ${BIN_HOME} 不在 PATH 中"
	say "请把以下内容加入 shell 配置:"
	say "  export PATH=\"${BIN_HOME}:\$PATH\""
	;;
esac
