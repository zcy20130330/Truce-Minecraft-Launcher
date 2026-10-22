using System.Collections.Concurrent;
using System.Collections.ObjectModel;
using System.ComponentModel;
using System.IO;
using System.Windows;
using System.Windows.Data;
using System.Windows.Threading;
using Microsoft.Win32;
using TruceLauncher.Models;
using TruceLauncher.Services;

namespace TruceLauncher;

public sealed record LanguageOption(string Value, string Label)
{
    public override string ToString() => Label;
}

public sealed class LogLine
{
    public string Time { get; init; } = "";
    public string Text { get; init; } = "";
    public bool IsError { get; init; }
}

public sealed class MainViewModel : ObservableObject
{
    private const int MaxLogLines = 4000;

    private readonly ConcurrentQueue<LogLine> _pending = new();
    private readonly DispatcherTimer _logTimer;
    private LauncherConfig _cfg = new();
    private CancellationTokenSource? _launchCts;

    public MainViewModel()
    {
        VersionsView = CollectionViewSource.GetDefaultView(Versions);
        VersionsView.Filter = FilterVersion;

        _logTimer = new DispatcherTimer(DispatcherPriority.Background)
        {
            Interval = TimeSpan.FromMilliseconds(110)
        };
        _logTimer.Tick += (_, _) => FlushLogs();
        _logTimer.Start();

        RefreshCommand = new AsyncRelayCommand(RefreshAsync);
        LaunchCommand = new AsyncRelayCommand(LaunchAsync);
        SaveCommand = new RelayCommand(SaveConfigAndToast);
        BrowseMcCommand = new RelayCommand(BrowseMcFolder);
        BrowseJavaCommand = new AsyncRelayCommand(BrowseJava);
        AutoInstallJavaCommand = new AsyncRelayCommand(AutoInstallJavaAsync);
        OptimizeCommand = new AsyncRelayCommand(OptimizeAsync);
        OpenMcFolderCommand = new RelayCommand(() => CoreClient.OpenFolder(McFolder));
        ClearLogCommand = new RelayCommand(() => Logs.Clear());
        ToggleConsoleCommand = new RelayCommand(() => IsConsoleOpen = !IsConsoleOpen);
    }

    // ---------- 命令 ----------
    public AsyncRelayCommand RefreshCommand { get; }
    public AsyncRelayCommand LaunchCommand { get; }
    public RelayCommand SaveCommand { get; }
    public RelayCommand BrowseMcCommand { get; }
    public AsyncRelayCommand BrowseJavaCommand { get; }
    public AsyncRelayCommand AutoInstallJavaCommand { get; }
    public AsyncRelayCommand OptimizeCommand { get; }
    public RelayCommand OpenMcFolderCommand { get; }
    public RelayCommand ClearLogCommand { get; }
    public RelayCommand ToggleConsoleCommand { get; }

    // ---------- 状态 ----------
    private string _statusText = "正在启动…";
    public string StatusText { get => _statusText; set => Set(ref _statusText, value); }

    private string _statusDetail = "";
    public string StatusDetail { get => _statusDetail; set => Set(ref _statusDetail, value); }

    private bool _isBusy = true;
    public bool IsBusy
    {
        get => _isBusy;
        set { if (Set(ref _isBusy, value)) OnPropertyChanged(nameof(CanInteract)); }
    }

    private bool _isLaunching;
    public bool IsLaunching
    {
        get => _isLaunching;
        set
        {
            if (!Set(ref _isLaunching, value)) return;
            OnPropertyChanged(nameof(LaunchButtonText));
            OnPropertyChanged(nameof(CanInteract));
        }
    }

    public bool CanInteract => !IsBusy && !IsLaunching;
    public string LaunchButtonText => IsLaunching ? "游戏运行中…" : "启动游戏";

    private bool _isConsoleOpen = true;
    public bool IsConsoleOpen { get => _isConsoleOpen; set => Set(ref _isConsoleOpen, value); }

    public string AppVersion =>
        "v" + (System.Reflection.Assembly.GetExecutingAssembly().GetName().Version?.ToString(3) ?? "1.1.0");

    // ---------- 版本 ----------
    public ObservableCollection<VersionItem> Versions { get; } = new();
    public ICollectionView VersionsView { get; }

    private VersionItem? _selectedVersion;
    public VersionItem? SelectedVersion
    {
        get => _selectedVersion;
        set
        {
            if (!Set(ref _selectedVersion, value)) return;
            OnPropertyChanged(nameof(HasSelection));
            OnPropertyChanged(nameof(SelectedVersionTitle));
            UpdateJavaRecommendation();
            UpdateSelectionHint();
        }
    }

    public bool HasSelection => SelectedVersion != null;
    public string SelectedVersionTitle => SelectedVersion?.Name ?? "未选择版本";

    private string _searchText = "";
    public string SearchText
    {
        get => _searchText;
        set { if (Set(ref _searchText, value)) VersionsView.Refresh(); }
    }

    public int VersionCount => Versions.Count;

    // ---------- Java / 内存 ----------
    public ObservableCollection<JavaInfo> JavaList { get; } = new();

    private JavaInfo? _selectedJava;
    public JavaInfo? SelectedJava
    {
        get => _selectedJava;
        set
        {
            if (!Set(ref _selectedJava, value)) return;
            UpdateJavaRecommendation();
        }
    }

    /// <summary>当前版本要求的 Java 主版本（0 = 版本文件未标注）。</summary>
    private int _javaRequired;
    /// <summary>本机最合适的那个 Java（与核心 SelectJava 规则一致）。</summary>
    private JavaInfo? _javaBest;
    /// <summary>缺失时应自动安装的主版本。</summary>
    private int _installMajor = 21;

    /// <summary>本机缺少满足要求的 Java 时，界面上显示「自动安装」按钮。</summary>
    public bool NeedJavaInstall => _javaBest == null;

    public string InstallJavaText => $"自动安装 Java {_installMajor}";

    /// <summary>当前选中版本 + 当前 Java 选择状态的一句话说明。</summary>
    public string JavaHint
    {
        get
        {
            var sel = SelectedJava;
            if (sel == null) return "本机未检测到 Java";

            if (sel.IsAuto)
            {
                if (_javaBest == null)
                {
                    return _javaRequired > 0
                        ? $"该版本要求 Java {_javaRequired}，本机缺少可用版本"
                        : "本机未检测到 Java，建议一键安装";
                }
                return _javaRequired > 0 && _javaBest.Major != _javaRequired
                    ? $"该版本要求 Java {_javaRequired}，将自动使用 Java {_javaBest.Major}"
                    : $"已自动匹配 Java {_javaBest.Major}   ·   {_javaBest.Path}";
            }

            if (_javaRequired > 0 && sel.Major < _javaRequired)
                return $"Java {sel.Major} 低于该版本要求的 Java {_javaRequired}，启动可能失败";
            return sel.Detail;
        }
    }

    /// <summary>按当前选中版本重新计算 Java 推荐（规则与核心 java.Recommend 保持一致）：
    /// 精确同版本优先 → 更高的版本里最低的 → 都没有则判定需要安装。</summary>
    private void UpdateJavaRecommendation()
    {
        _javaRequired = SelectedVersion?.Java ?? 0;

        var installed = JavaList.Where(j => !j.IsAuto).ToList();
        JavaInfo? best;
        if (_javaRequired > 0)
        {
            best = installed.FirstOrDefault(j => j.Major == _javaRequired)
                   ?? installed.Where(j => j.Major > _javaRequired)
                               .OrderBy(j => j.Major)
                               .FirstOrDefault();
        }
        else
        {
            best = installed.OrderByDescending(j => j.Major).FirstOrDefault();
        }

        _javaBest = best;
        _installMajor = _javaRequired > 0 ? _javaRequired : (best?.Major ?? 21);

        OnPropertyChanged(nameof(JavaHint));
        OnPropertyChanged(nameof(NeedJavaInstall));
        OnPropertyChanged(nameof(InstallJavaText));
    }

    private MemoryInfo? _memory;
    public MemoryInfo? Memory
    {
        get => _memory;
        set
        {
            if (!Set(ref _memory, value)) return;
            OnPropertyChanged(nameof(MaxRamGb));
            OnPropertyChanged(nameof(SuggestMb));
            OnPropertyChanged(nameof(RamText));
            OnPropertyChanged(nameof(MemoryHint));
        }
    }

    public int SuggestMb => Memory?.SuggestMb ?? 4096;
    public double MaxRamGb => Math.Max(2, Math.Round(((Memory?.TotalMb ?? 8192) / 1024.0) * 0.9));

    public string MemoryHint => Memory == null
        ? ""
        : $"物理内存 {Memory.TotalMb} MB · 当前可用 {Memory.AvailableMb} MB";

    // ---------- 配置绑定 ----------
    private string _username = "Player";
    public string Username { get => _username; set => Set(ref _username, value); }

    private bool _autoMemory = true;
    public bool AutoMemory
    {
        get => _autoMemory;
        set { if (Set(ref _autoMemory, value)) OnPropertyChanged(nameof(RamText)); }
    }

    private double _ramGb = 4;
    public double RamGb
    {
        get => _ramGb;
        set
        {
            if (!Set(ref _ramGb, Math.Round(value, 1))) return;
            OnPropertyChanged(nameof(RamMb));
            OnPropertyChanged(nameof(RamText));
        }
    }

    public int RamMb => (int)Math.Round(RamGb * 1024);
    public string RamText => AutoMemory ? $"自动分配 · 推荐 {SuggestMb} MB" : $"手动分配 · {RamMb} MB";

    private bool _memoryOptimize = true;
    public bool MemoryOptimize { get => _memoryOptimize; set => Set(ref _memoryOptimize, value); }

    public IReadOnlyList<LanguageOption> Languages { get; } = new List<LanguageOption>
    {
        new("system", "跟随系统"),
        new("none", "不修改（游戏内设置）"),
        new("zh_cn", "简体中文"),
        new("zh_tw", "繁體中文"),
        new("en_us", "English"),
        new("ja_jp", "日本語"),
        new("ko_kr", "한국어"),
    };

    private LanguageOption _selectedLanguage = null!;
    public LanguageOption SelectedLanguage
    {
        get => _selectedLanguage;
        set { if (value != null) Set(ref _selectedLanguage, value); }
    }

    private int _windowWidth = 854;
    public int WindowWidth { get => _windowWidth; set => Set(ref _windowWidth, value); }

    private int _windowHeight = 480;
    public int WindowHeight { get => _windowHeight; set => Set(ref _windowHeight, value); }

    private bool _fullscreen;
    public bool Fullscreen { get => _fullscreen; set => Set(ref _fullscreen, value); }

    private bool _versionIsolate;
    public bool VersionIsolate { get => _versionIsolate; set => Set(ref _versionIsolate, value); }

    private string _jvmArgs = "";
    public string JvmArgs { get => _jvmArgs; set => Set(ref _jvmArgs, value); }

    private string _gameArgs = "";
    public string GameArgs { get => _gameArgs; set => Set(ref _gameArgs, value); }

    private string _mcFolder = "";
    public string McFolder { get => _mcFolder; set => Set(ref _mcFolder, value); }

    // ---------- 日志 ----------
    public ObservableCollection<LogLine> Logs { get; } = new();

    // ---------- 初始化 ----------
    public async Task InitializeAsync()
    {
        SelectedLanguage ??= Languages[0];
        IsBusy = true;
        StatusText = "正在读取本地环境…";
        StatusDetail = "检测 Java / 内存 / 已安装版本";

        try
        {
            AddLog($"[启动器] 核心进程：{CoreClient.CorePath}");
            var probe = await CoreClient.ProbeAsync();
            ApplyProbe(probe);

            StatusText = Versions.Count > 0 ? "就绪" : "未找到游戏版本";
            StatusDetail = Versions.Count > 0
                ? $"已找到 {Versions.Count} 个版本 · {JavaList.Count} 个 Java · 物理内存 {Memory?.TotalMb ?? 0} MB"
                : $"请检查 .minecraft 目录：{McFolder}";

            AddLog($"[启动器] 已加载 {Versions.Count} 个版本、{JavaList.Count} 个 Java 运行时");
        }
        catch (Exception ex)
        {
            StatusText = "初始化失败";
            StatusDetail = ex.Message;
            AddLog("[错误] " + ex.Message, true);
        }
        finally
        {
            IsBusy = false;
        }
    }

    private void ApplyProbe(ProbeResult probe)
    {
        _cfg = probe.Config ?? new LauncherConfig();

        Versions.Clear();
        foreach (var v in probe.Versions) Versions.Add(v);
        OnPropertyChanged(nameof(VersionCount));

        JavaList.Clear();
        JavaList.Add(JavaInfo.Auto); // 首项：自动选择（交给核心按版本需求匹配）
        foreach (var j in probe.Java) JavaList.Add(j);

        // 配置里明确指定过就沿用具体路径，否则默认「自动选择」
        SelectedJava =
            JavaList.FirstOrDefault(j => !j.IsAuto
                                         && !string.IsNullOrEmpty(_cfg.JavaPath)
                                         && string.Equals(j.Path, _cfg.JavaPath, StringComparison.OrdinalIgnoreCase))
            ?? JavaInfo.Auto;
        UpdateJavaRecommendation();

        Memory = probe.Memory;
        McFolder = string.IsNullOrWhiteSpace(probe.McFolder) ? _cfg.McFolder : probe.McFolder;

        Username = string.IsNullOrWhiteSpace(_cfg.Username) ? "Player" : _cfg.Username;
        AutoMemory = _cfg.AutoMemory;
        RamGb = _cfg.RamGB <= 0 ? 4 : _cfg.RamGB;
        MemoryOptimize = _cfg.MemoryOptimize;
        SelectedLanguage = Languages.FirstOrDefault(l => l.Value == _cfg.Language) ?? Languages[0];
        WindowWidth = _cfg.WindowWidth > 0 ? _cfg.WindowWidth : 854;
        WindowHeight = _cfg.WindowHeight > 0 ? _cfg.WindowHeight : 480;
        Fullscreen = _cfg.Fullscreen;
        VersionIsolate = _cfg.VersionIsolate;
        JvmArgs = _cfg.JvmArgs;
        GameArgs = _cfg.GameArgs;

        SelectedVersion = Versions.FirstOrDefault();
    }

    private void UpdateSelectionHint()
    {
        if (SelectedVersion == null) return;
        var need = SelectedVersion.Java;
        var has = SelectedJava?.Major ?? 0;
        if (need > 0 && has > 0 && has != need)
        {
            StatusDetail = $"该版本建议 Java {need}，当前选择 Java {has}";
        }
    }

    // ---------- 刷新 ----------
    private async Task RefreshAsync()
    {
        IsBusy = true;
        StatusText = "正在扫描版本…";
        try
        {
            var keep = SelectedVersion?.Name;
            var list = await CoreClient.ScanAsync();
            Versions.Clear();
            foreach (var v in list) Versions.Add(v);
            OnPropertyChanged(nameof(VersionCount));

            SelectedVersion = Versions.FirstOrDefault(v => v.Name == keep) ?? Versions.FirstOrDefault();
            StatusText = "就绪";
            StatusDetail = $"已找到 {Versions.Count} 个版本";
            AddLog($"[启动器] 刷新完成：{Versions.Count} 个版本");
        }
        catch (Exception ex)
        {
            StatusText = "扫描失败";
            StatusDetail = ex.Message;
            AddLog("[错误] " + ex.Message, true);
        }
        finally
        {
            IsBusy = false;
        }
    }

    // ---------- 启动 ----------
    private async Task LaunchAsync()
    {
        if (SelectedVersion == null || IsLaunching) return;

        SaveConfig();
        IsLaunching = true;
        IsConsoleOpen = true;
        StatusText = $"正在启动 {SelectedVersion.Name}…";
        var javaText = SelectedJava == null || SelectedJava.IsAuto
            ? (_javaBest != null ? $"自动 → Java {_javaBest.Major}" : "自动")
            : $"Java {SelectedJava.Major}";
        StatusDetail = $"{javaText} · 内存 {(AutoMemory ? SuggestMb : RamMb)} MB";
        AddLog($"[启动器] 启动 {SelectedVersion.Name}，内存 {(AutoMemory ? SuggestMb : RamMb)} MB");

        _launchCts = new CancellationTokenSource();
        try
        {
            var (ok, msg) = await CoreClient.LaunchAsync(
                SelectedVersion.Name, line => Enqueue(line), _launchCts.Token);

            StatusText = ok ? "游戏已退出" : "启动失败";
            StatusDetail = string.IsNullOrWhiteSpace(msg) ? "" : msg;
            AddLog(ok ? "[启动器] 游戏已退出" : $"[启动器] 启动失败：{msg}", !ok);
        }
        catch (OperationCanceledException)
        {
            StatusText = "已取消";
        }
        catch (Exception ex)
        {
            StatusText = "启动失败";
            StatusDetail = ex.Message;
            AddLog("[错误] " + ex.Message, true);
        }
        finally
        {
            IsLaunching = false;
            _launchCts?.Dispose();
            _launchCts = null;
        }
    }

    // ---------- 内存优化 ----------
    private async Task OptimizeAsync()
    {
        IsConsoleOpen = true;
        StatusText = "正在优化内存…";
        StatusDetail = "整理工作集与待机列表（需管理员权限才完整生效）";
        try
        {
            var r = await CoreClient.OptimizeAsync(true, line => Enqueue(line), CancellationToken.None);
            if (r != null)
            {
                StatusText = r.Ok ? $"内存优化完成 · 释放 {r.FreedMb} MB" : "内存优化已执行";
                StatusDetail = $"可用 {r.BeforeMb} MB → {r.AfterMb} MB" + (r.Admin ? "" : "（未以管理员身份运行，部分步骤被系统拒绝）");
            }
            else
            {
                StatusText = "内存优化已执行";
            }
        }
        catch (Exception ex)
        {
            StatusText = "内存优化失败";
            StatusDetail = ex.Message;
            AddLog("[错误] " + ex.Message, true);
        }
    }

    // ---------- 浏览 ----------
    private void BrowseMcFolder()
    {
        var dlg = new OpenFolderDialog
        {
            Title = "选择 .minecraft 目录",
            Multiselect = false,
        };
        try
        {
            if (Directory.Exists(McFolder)) dlg.InitialDirectory = McFolder;
        }
        catch { /* ignore */ }

        if (dlg.ShowDialog() != true) return;
        McFolder = dlg.FolderName;
        _cfg.McFolder = McFolder;
        CoreClient.SaveConfig(_cfg);
        _ = RefreshAsync();
    }

    private async Task BrowseJava()
    {
        var dlg = new OpenFileDialog
        {
            Title = "选择 java.exe",
            Filter = "java.exe|java.exe|可执行文件|*.exe|所有文件|*.*",
            CheckFileExists = true,
        };
        if (dlg.ShowDialog() != true) return;

        try
        {
            var info = await CoreClient.CheckJavaAsync(dlg.FileName);
            if (info == null || info.Major <= 0)
            {
                StatusText = "该文件不是可用的 Java";
                AddLog($"[错误] 无法识别 Java：{dlg.FileName}", true);
                return;
            }

            var exist = JavaList.FirstOrDefault(j =>
                string.Equals(j.Path, info.Path, StringComparison.OrdinalIgnoreCase));
            if (exist == null) JavaList.Insert(0, info);
            SelectedJava = exist ?? info;
            _cfg.JavaPath = info.Path;
            CoreClient.SaveConfig(_cfg);
            StatusText = $"已选择 Java {info.Major}";
        }
        catch (Exception ex)
        {
            AddLog("[错误] " + ex.Message, true);
        }
    }

    // ---------- Java 自动安装 ----------
    private async Task AutoInstallJavaAsync()
    {
        if (_javaBest != null) return; // 已有满足要求的 Java，无需安装

        var major = _installMajor;
        IsBusy = true;
        IsConsoleOpen = true;
        StatusText = $"正在安装 Java {major}…";
        StatusDetail = "准备获取 Temurin JRE";
        AddLog($"[Java] 开始自动安装 Java {major}");

        var lastPct = -1;
        var lastTick = DateTime.MinValue;

        try
        {
            var (ok, path, error) = await CoreClient.InstallJavaAsync(major,
                p =>
                {
                    if (p.Stage == "download")
                    {
                        if (p.Percent < 0)
                        {
                            // 断流重试：进度不推进，只更新文字说明
                            StatusText = $"正在重试下载 Java {major}…";
                            StatusDetail = p.Message;
                            return;
                        }
                        var now = DateTime.UtcNow;
                        if (p.Percent == lastPct && (now - lastTick).TotalMilliseconds < 800) return;
                        lastPct = p.Percent;
                        lastTick = now;
                        StatusText = $"正在下载 Java {major} · {p.Percent}%";
                        StatusDetail = p.Message;
                        return;
                    }

                    StatusText = p.Stage switch
                    {
                        "resolve" => $"正在解析 Java {major} 下载源…",
                        "extract" => $"正在解压 Java {major}…",
                        "verify" => $"正在校验 Java {major}…",
                        "done" => $"Java {major} 安装完成",
                        _ => StatusText
                    };
                    StatusDetail = p.Message;
                },
                line => Enqueue("[Java] " + line),
                CancellationToken.None);

            if (!ok)
            {
                StatusText = $"Java {major} 安装失败";
                StatusDetail = string.IsNullOrWhiteSpace(error) ? "未知错误" : error;
                AddLog($"[错误] Java {major} 安装失败：{error}", true);
                return;
            }

            var info = await CoreClient.CheckJavaAsync(path);
            if (info != null && info.Major > 0)
            {
                var exist = JavaList.FirstOrDefault(j => !j.IsAuto
                    && string.Equals(j.Path, info.Path, StringComparison.OrdinalIgnoreCase));
                if (exist == null) JavaList.Add(info);
            }

            UpdateJavaRecommendation();
            StatusText = $"Java {major} 安装完成";
            StatusDetail = path;
            AddLog($"[Java] 已安装：{path}");
        }
        catch (Exception ex)
        {
            StatusText = "Java 安装失败";
            StatusDetail = ex.Message;
            AddLog("[错误] " + ex.Message, true);
        }
        finally
        {
            IsBusy = false;
            UpdateJavaRecommendation();
        }
    }

    // ---------- 保存 ----------
    private void SaveConfigAndToast()
    {
        SaveConfig();
        StatusText = "设置已保存";
        StatusDetail = CoreClient.ConfigPath;
        AddLog("[启动器] 设置已写入 " + CoreClient.ConfigPath);
    }

    public void SaveConfig()
    {
        _cfg.McFolder = McFolder;
        // 「自动选择」写空路径，核心会按版本需求自行匹配
        _cfg.JavaPath = SelectedJava == null || SelectedJava.IsAuto ? "" : SelectedJava.Path;
        _cfg.RamGB = Math.Round(RamGb, 1);
        _cfg.AutoMemory = AutoMemory;
        _cfg.MemoryOptimize = MemoryOptimize;
        _cfg.Language = SelectedLanguage?.Value ?? "system";
        _cfg.Username = string.IsNullOrWhiteSpace(Username) ? "Player" : Username.Trim();
        _cfg.WindowWidth = WindowWidth;
        _cfg.WindowHeight = WindowHeight;
        _cfg.Fullscreen = Fullscreen;
        _cfg.VersionIsolate = VersionIsolate;
        _cfg.JvmArgs = JvmArgs;
        _cfg.GameArgs = GameArgs;
        CoreClient.SaveConfig(_cfg);
    }

    // ---------- 日志 ----------
    private void Enqueue(string text, bool isError = false)
    {
        _pending.Enqueue(new LogLine
        {
            Time = DateTime.Now.ToString("HH:mm:ss"),
            Text = text,
            IsError = isError
        });
    }

    private void AddLog(string text, bool isError = false)
    {
        Logs.Add(new LogLine { Time = DateTime.Now.ToString("HH:mm:ss"), Text = text, IsError = isError });
        if (Logs.Count > MaxLogLines) Logs.RemoveAt(0);
    }

    private void FlushLogs()
    {
        int n = 0;
        while (n < 300 && _pending.TryDequeue(out var line))
        {
            Logs.Add(line);
            n++;
        }
        while (Logs.Count > MaxLogLines) Logs.RemoveAt(0);
    }

    // ---------- 过滤 ----------
    private bool FilterVersion(object obj)
    {
        if (string.IsNullOrWhiteSpace(SearchText)) return true;
        if (obj is not VersionItem v) return false;
        var q = SearchText.Trim();
        return v.Name.Contains(q, StringComparison.OrdinalIgnoreCase)
               || v.Loaders.Contains(q, StringComparison.OrdinalIgnoreCase)
               || v.TypeLabel.Contains(q);
    }
}
