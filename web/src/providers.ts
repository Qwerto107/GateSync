export const providers = [
  { value: "tencent", label: "腾讯云" },
  { value: "aliyun", label: "阿里云" },
  { value: "aws", label: "AWS" },
];

export function providerName(provider?: string) {
  return providers.find((p) => p.value === provider)?.label || "腾讯云";
}

export function resourceOptions(provider?: string) {
  if (provider === "aws") {
    return [{ value: "security-group", label: "EC2 / VPC 安全组" }];
  }
  return provider === "aliyun"
    ? [
        { value: "security-group", label: "ECS 安全组" },
        { value: "swas", label: "轻量应用服务器防火墙" },
      ]
    : [
        { value: "security-group", label: "VPC 安全组" },
        { value: "lighthouse", label: "Lighthouse 防火墙" },
      ];
}

export function defaultRegion(provider?: string) {
  if (provider === "aliyun") return "cn-hangzhou";
  if (provider === "aws") return "us-east-1";
  return "ap-guangzhou";
}

export function resourceName(provider: string | undefined, kind: string) {
  return (
    resourceOptions(provider).find((r) => r.value === kind)?.label || "安全组"
  );
}
