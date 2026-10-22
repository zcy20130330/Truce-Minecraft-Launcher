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
        set { if (Set(ref _selectedJava, value)) OnPropertyChanged(nameof(JavaHint)); }
    }

    public string JavaHint => SelectedJava?.Detail ?? "未检测到 Java，请手动指定 java.exe";

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
        foreach (var j in probe.Java) JavaList.Add(j);
        SelectedJava =
            JavaList.FirstOrDefault(j => !string.IsNullOrEmpty(_cfg.JavaPath) &&
                                         string.Equals(j.Path, _cfg.JavaPath, StringComparison.OrdinalIgnoreCase))
            ?? JavaList.FirstOrDefault(j => j.Major == probe.SelectedJava?.Major)
            ?? JavaList.FirstOrDefault();

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
        StatusDetail = $"Java {(SelectedJava?.Major.ToString() ?? "自动")} · 内存 {(AutoMemory ? SuggestMb : RamMb)} MB";
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
        _cfg.JavaPath = SelectedJava?.Path ?? "";
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
