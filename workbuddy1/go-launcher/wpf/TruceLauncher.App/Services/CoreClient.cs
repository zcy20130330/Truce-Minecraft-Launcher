using System.Diagnostics;
using System.IO;
using System.Text;
using System.Text.Encodings.Web;
using System.Text.Json;
using TruceLauncher.Models;

namespace TruceLauncher.Services;

/// <summary>
/// Go 核心进程的封装：负责把内嵌的 core.exe 释放到本地目录，
/// 并以子进程方式调用 probe / scan / launch / optimize。
/// </summary>
public static class CoreClient
{
    public static string DataDir { get; } = Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "TruceLauncher");

    public static string ConfigPath { get; } = Path.Combine(DataDir, "config.json");

    private static string? _corePath;
    public static string CorePath => _corePath ??= ExtractCore();

    private static readonly JsonSerializerOptions ReadOpts = new()
    {
        PropertyNameCaseInsensitive = true,
        NumberHandling = System.Text.Json.Serialization.JsonNumberHandling.AllowReadingFromString,
    };

    private static readonly JsonSerializerOptions WriteOpts = new()
    {
        WriteIndented = true,
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    // ---------- 核心释放 ----------

    private static string ExtractCore()
    {
        Directory.CreateDirectory(DataDir);
        var dir = Path.Combine(DataDir, "core");
        Directory.CreateDirectory(dir);

        var asm = typeof(CoreClient).Assembly;
        var ver = asm.GetName().Version?.ToString() ?? "1.0.0.0";
        var target = Path.Combine(dir, $"core-{ver}.exe");

        using var res = asm.GetManifestResourceStream("core.exe")
            ?? throw new InvalidOperationException("内嵌核心 core.exe 未找到，构建可能不完整。");

        if (File.Exists(target) && new FileInfo(target).Length == res.Length)
            return target;

        // 清理历史版本的核心文件（被占用则跳过）
        foreach (var old in Directory.GetFiles(dir, "core-*.exe"))
        {
            if (string.Equals(old, target, StringComparison.OrdinalIgnoreCase)) continue;
            try { File.Delete(old); } catch { /* ignore */ }
        }

        var tmp = target + ".tmp";
        using (var fs = new FileStream(tmp, FileMode.Create, FileAccess.Write, FileShare.None))
            res.CopyTo(fs);
        try { if (File.Exists(target)) File.Delete(target); } catch { /* ignore */ }
        File.Move(tmp, target, true);
        return target;
    }

    // ---------- 进程调用 ----------

    private static ProcessStartInfo BuildPsi(string args) => new()
    {
        FileName = CorePath,
        Arguments = args,
        WorkingDirectory = DataDir,
        UseShellExecute = false,
        CreateNoWindow = true,
        RedirectStandardOutput = true,
        RedirectStandardError = true,
        StandardOutputEncoding = new UTF8Encoding(false),
        StandardErrorEncoding = new UTF8Encoding(false),
    };

    private static string Q(string s) => "\"" + s.Replace("\"", "\\\"") + "\"";

    private static async Task<(int Code, string Out, string Err)> RunCaptureAsync(
        string args, int timeoutMs, CancellationToken ct)
    {
        using var p = new Process { StartInfo = BuildPsi(args) };
        if (!p.Start()) throw new InvalidOperationException("无法启动核心进程。");

        var so = p.StandardOutput.ReadToEndAsync(ct);
        var se = p.StandardError.ReadToEndAsync(ct);

        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(timeoutMs);
        try
        {
            await p.WaitForExitAsync(cts.Token);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            try { p.Kill(true); } catch { /* ignore */ }
            throw new TimeoutException("核心进程响应超时。");
        }

        return (p.ExitCode, await so, await se);
    }

    /// <summary>流式运行（launch / optimize），逐行回调输出。</summary>
    public static async Task<int> RunStreamingAsync(string args, Action<string> onLine, CancellationToken ct)
    {
        using var p = new Process { StartInfo = BuildPsi(args) };
        p.OutputDataReceived += (_, e) => { if (e.Data is not null) onLine(e.Data); };
        p.ErrorDataReceived += (_, e) => { if (e.Data is not null) onLine(e.Data); };
        p.Start();
        p.BeginOutputReadLine();
        p.BeginErrorReadLine();
        await p.WaitForExitAsync(ct);
        return p.ExitCode;
    }

    // ---------- 业务接口 ----------

    public static async Task<ProbeResult> ProbeAsync(CancellationToken ct = default)
    {
        var (code, stdout, stderr) = await RunCaptureAsync(
            $"probe --config {Q(ConfigPath)}", 120_000, ct);

        var text = (stdout ?? "").Trim();
        int brace = text.IndexOf('{');
        if (brace > 0) text = text[brace..];

        if (string.IsNullOrWhiteSpace(text))
            throw new InvalidOperationException(
                $"核心未返回数据（退出码 {code}）。{stderr}".Trim());

        var result = JsonSerializer.Deserialize<ProbeResult>(text, ReadOpts);
        if (result == null) throw new InvalidOperationException("核心返回的数据无法解析。");
        return result;
    }

    public static async Task<List<VersionItem>> ScanAsync(CancellationToken ct = default)
    {
        var (_, stdout, _) = await RunCaptureAsync($"scan --config {Q(ConfigPath)}", 60_000, ct);
        var text = (stdout ?? "").Trim();
        int bracket = text.IndexOf('[');
        if (bracket > 0) text = text[bracket..];
        if (string.IsNullOrWhiteSpace(text)) return new List<VersionItem>();
        return JsonSerializer.Deserialize<List<VersionItem>>(text, ReadOpts) ?? new List<VersionItem>();
    }

    /// <summary>启动游戏。返回是否成功（依据核心输出中的“结果: ok=true”）。</summary>
    public static async Task<(bool Ok, string Message)> LaunchAsync(
        string version, Action<string> onLine, CancellationToken ct)
    {
        bool ok = false;
        string message = "";
        var code = await RunStreamingAsync($"launch --config {Q(ConfigPath)} --version {Q(version)}",
            line =>
            {
                onLine(line);
                if (line.Contains("=== 结果:"))
                {
                    ok = line.Contains("ok=true");
                    message = line.Trim();
                }
            }, ct);
        if (code != 0 && message.Length == 0) message = $"核心退出码 {code}";
        return (ok, message);
    }

    public static async Task<OptimizeResult?> OptimizeAsync(        bool perProcess, Action<string> onLine, CancellationToken ct)
    {
        OptimizeResult? result = null;
        var args = $"optimize --config {Q(ConfigPath)}" + (perProcess ? " --per-process" : "");
        await RunStreamingAsync(args, line =>
        {
            const string marker = "###RESULT###";
            if (line.StartsWith(marker, StringComparison.Ordinal))
            {
                try
                {
                    result = JsonSerializer.Deserialize<OptimizeResult>(line[marker.Length..], ReadOpts);
                }
                catch { /* 摘要解析失败不影响日志 */ }
                return;
            }
            onLine(line);
        }, ct);
        return result;
    }

    /// <summary>校验一个 java.exe 路径；无法识别时返回 null。</summary>
    public static async Task<JavaInfo?> CheckJavaAsync(string path, CancellationToken ct = default)
    {
        var (_, stdout, _) = await RunCaptureAsync($"javacheck --path {Q(path)}", 20_000, ct);
        var text = (stdout ?? "").Trim();
        int brace = text.IndexOf('{');
        if (brace > 0) text = text[brace..];
        if (string.IsNullOrWhiteSpace(text)) return null;

        var info = JsonSerializer.Deserialize<JavaInfo>(text, ReadOpts);
        return info != null && info.Major > 0 ? info : null;
    }

    /// <summary>自动选择结论：某个版本该用哪个 Java、是否需要安装。</summary>
    public static async Task<JavaFindResult?> FindJavaAsync(
        string versionName, CancellationToken ct = default)
    {
        var (_, stdout, _) = await RunCaptureAsync(
            $"javafind --config {Q(ConfigPath)} --version {Q(versionName)}", 90_000, ct);
        var text = (stdout ?? "").Trim();
        int brace = text.IndexOf('{');
        if (brace > 0) text = text[brace..];
        if (string.IsNullOrWhiteSpace(text)) return null;
        try
        {
            return JsonSerializer.Deserialize<JavaFindResult>(text, ReadOpts);
        }
        catch
        {
            return null;
        }
    }

    /// <summary>自动下载并安装指定主版本的 JRE，返回 java.exe 路径。</summary>
    public static async Task<(bool Ok, string JavaPath, string Error)> InstallJavaAsync(
        int major, Action<JavaInstallProgress> onProgress, Action<string> onLine, CancellationToken ct)
    {
        bool ok = false;
        string javaPath = "", error = "";

        await RunStreamingAsync($"javainstall --major {major}", line =>
        {
            const string pm = "###PROGRESS###";
            const string rm = "###RESULT###";

            if (line.StartsWith(pm, StringComparison.Ordinal))
            {
                try
                {
                    var p = JsonSerializer.Deserialize<JavaInstallProgress>(line[pm.Length..], ReadOpts);
                    if (p != null) onProgress(p);
                }
                catch { /* 单条进度解析失败忽略 */ }
                return;
            }

            if (line.StartsWith(rm, StringComparison.Ordinal))
            {
                try
                {
                    using var doc = JsonDocument.Parse(line[rm.Length..]);
                    var root = doc.RootElement;
                    ok = root.TryGetProperty("ok", out var okEl) && okEl.GetBoolean();
                    if (root.TryGetProperty("java_path", out var pEl)) javaPath = pEl.GetString() ?? "";
                    if (root.TryGetProperty("error", out var eEl)) error = eEl.GetString() ?? "";
                }
                catch { /* ignore */ }
                return;
            }

            if (!string.IsNullOrWhiteSpace(line)) onLine(line);
        }, ct);

        return (ok, javaPath, error);
    }

    public static void SaveConfig(LauncherConfig cfg)
    {
        Directory.CreateDirectory(DataDir);
        File.WriteAllText(ConfigPath, JsonSerializer.Serialize(cfg, WriteOpts), new UTF8Encoding(false));
    }

    public static void OpenFolder(string path)
    {
        try
        {
            if (!string.IsNullOrWhiteSpace(path)) Directory.CreateDirectory(path);
            Process.Start(new ProcessStartInfo("explorer.exe", Q(path)) { UseShellExecute = true });
        }
        catch { /* ignore */ }
    }
}
