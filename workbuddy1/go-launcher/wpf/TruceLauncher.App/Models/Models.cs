using System.Globalization;
using System.Text.Json.Serialization;

namespace TruceLauncher.Models;

/// <summary>与 Go 核心 config.json 完全一致的配置结构（snake_case）。</summary>
public sealed class LauncherConfig
{
    [JsonPropertyName("mc_folder")] public string McFolder { get; set; } = "";
    [JsonPropertyName("java_path")] public string JavaPath { get; set; } = "";
    [JsonPropertyName("ram_gb")] public double RamGB { get; set; } = 4;
    [JsonPropertyName("auto_memory")] public bool AutoMemory { get; set; } = true;
    [JsonPropertyName("memory_optimize")] public bool MemoryOptimize { get; set; } = true;
    [JsonPropertyName("memory_reserve_mb")] public int MemoryReserveMB { get; set; } = 1024;
    [JsonPropertyName("language")] public string Language { get; set; } = "system";
    [JsonPropertyName("username")] public string Username { get; set; } = "Player";
    [JsonPropertyName("window_width")] public int WindowWidth { get; set; } = 854;
    [JsonPropertyName("window_height")] public int WindowHeight { get; set; } = 480;
    [JsonPropertyName("fullscreen")] public bool Fullscreen { get; set; }
    [JsonPropertyName("version_isolate")] public bool VersionIsolate { get; set; }
    [JsonPropertyName("jvm_args")] public string JvmArgs { get; set; } = "";
    [JsonPropertyName("game_args")] public string GameArgs { get; set; } = "";
    [JsonPropertyName("server")] public string Server { get; set; } = "";
    [JsonPropertyName("download_source")] public int DownloadSource { get; set; }
    [JsonPropertyName("check_files")] public bool CheckFiles { get; set; } = true;
}

/// <summary>单个已安装版本（来自 core.exe probe/scan 的 versions 数组）。</summary>
public sealed class VersionItem
{
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("type")] public string Type { get; set; } = "unknown";
    [JsonPropertyName("release_time")] public string ReleaseTime { get; set; } = "";
    [JsonPropertyName("major")] public int Major { get; set; }
    [JsonPropertyName("loaders")] public string Loaders { get; set; } = "";
    [JsonPropertyName("java")] public int Java { get; set; }
    [JsonPropertyName("state")] public string State { get; set; } = "ok";
    [JsonPropertyName("error")] public string? Error { get; set; }

    [JsonIgnore] public bool IsError => State == "error";
    [JsonIgnore] public bool HasLoaders => LoaderTags.Length > 0;

    [JsonIgnore]
    public string[] LoaderTags => string.IsNullOrWhiteSpace(Loaders)
        ? Array.Empty<string>()
        : Loaders.Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);

    [JsonIgnore]
    public string TypeLabel => Type switch
    {
        "release" => "正式版",
        "snapshot" => "快照",
        "old_beta" => "Beta 旧版",
        "old_alpha" => "Alpha 旧版",
        "error" => "损坏",
        _ => "未知"
    };

    /// <summary>卡片副标题：正式版 · Java 21 · 2025-12-09</summary>
    [JsonIgnore]
    public string MetaLine
    {
        get
        {
            var parts = new List<string> { TypeLabel };
            if (Java > 0) parts.Add($"Java {Java}");
            if (DateTime.TryParse(ReleaseTime, CultureInfo.InvariantCulture,
                    DateTimeStyles.AdjustToUniversal, out var dt))
                parts.Add(dt.ToLocalTime().ToString("yyyy-MM-dd"));
            return string.Join("  ·  ", parts);
        }
    }
}

public sealed class JavaInfo
{
    [JsonPropertyName("path")] public string Path { get; set; } = "";
    [JsonPropertyName("version")] public int[] Version { get; set; } = Array.Empty<int>();
    [JsonPropertyName("major")] public int Major { get; set; }

    /// <summary>「自动选择」占位项：Path 为空、无主版本号，交给核心按版本需求匹配。</summary>
    [JsonIgnore] public bool IsAuto => Major <= 0 && string.IsNullOrWhiteSpace(Path);

    public const string AutoText = "自动选择（按版本匹配）";

    /// <summary>下拉框首项。</summary>
    public static JavaInfo Auto { get; } = new();

    [JsonIgnore] public string ShortLabel => IsAuto ? "自动选择" : $"Java {Major}";

    /// <summary>下拉框 / 选中项的单行展示文本。</summary>
    [JsonIgnore]
    public string Display => IsAuto
        ? AutoText
        : Major > 0 ? $"Java {Major}    ·    {Path}" : Path;

    [JsonIgnore]
    public string Detail
    {
        get
        {
            if (IsAuto) return AutoText;
            var ver = Version.Length > 0 ? string.Join(".", Version) : Major.ToString();
            return $"Java {ver}   ·   {Path}";
        }
    }
}

public sealed class MemoryInfo
{
    [JsonPropertyName("total_mb")] public int TotalMb { get; set; }
    [JsonPropertyName("available_mb")] public int AvailableMb { get; set; }
    [JsonPropertyName("suggest_mb")] public int SuggestMb { get; set; }
}

public sealed class ProbeResult
{
    [JsonPropertyName("config_path")] public string ConfigPath { get; set; } = "";
    [JsonPropertyName("config")] public LauncherConfig? Config { get; set; }
    [JsonPropertyName("mc_folder")] public string McFolder { get; set; } = "";
    [JsonPropertyName("mc_folder_exists")] public bool McFolderExists { get; set; }
    [JsonPropertyName("java")] public List<JavaInfo> Java { get; set; } = new();
    [JsonPropertyName("java_dir")] public string JavaDir { get; set; } = "";
    [JsonPropertyName("java_required")] public int JavaRequired { get; set; }
    [JsonPropertyName("selected_java")] public JavaInfo? SelectedJava { get; set; }
    [JsonPropertyName("system_language")] public string SystemLanguage { get; set; } = "zh_cn";
    [JsonPropertyName("versions")] public List<VersionItem> Versions { get; set; } = new();
    [JsonPropertyName("memory")] public MemoryInfo? Memory { get; set; }
}

/// <summary>内存优化结果摘要（解析 core.exe optimize 的 ###RESULT### 行）。</summary>
public sealed class OptimizeResult
{
    [JsonPropertyName("ok")] public bool Ok { get; set; }
    [JsonPropertyName("admin")] public bool Admin { get; set; }
    [JsonPropertyName("before_mb")] public int BeforeMb { get; set; }
    [JsonPropertyName("after_mb")] public int AfterMb { get; set; }
    [JsonPropertyName("freed_mb")] public int FreedMb { get; set; }
}

/// <summary>Java 自动安装的进度（解析 core.exe javainstall 的 ###PROGRESS### 行）。</summary>
public sealed class JavaInstallProgress
{
    [JsonPropertyName("stage")] public string Stage { get; set; } = "";
    [JsonPropertyName("percent")] public int Percent { get; set; }
    [JsonPropertyName("message")] public string Message { get; set; } = "";
}

/// <summary>javafind 的返回：某版本要求的 Java 与本机匹配结论。</summary>
public sealed class JavaFindResult
{
    [JsonPropertyName("version")] public string Version { get; set; } = "";
    [JsonPropertyName("java_dir")] public string JavaDir { get; set; } = "";
    [JsonPropertyName("available")] public List<JavaInfo> Available { get; set; } = new();
    [JsonPropertyName("required")] public int Required { get; set; }
    [JsonPropertyName("best")] public JavaInfo? Best { get; set; }
    [JsonPropertyName("need_install")] public bool NeedInstall { get; set; }
    [JsonPropertyName("install_major")] public int InstallMajor { get; set; }
    [JsonPropertyName("reason")] public string Reason { get; set; } = "";
}
