using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using System.Windows.Threading;

namespace TruceLauncher;

public partial class MainWindow : Window
{
    private readonly MainViewModel _vm = new();
    private bool _autoScrollPending;

    public MainWindow()
    {
        InitializeComponent();
        DataContext = _vm;

        Loaded += OnLoaded;
        StateChanged += (_, _) => UpdateMaximizeGlyph();
        _vm.Logs.CollectionChanged += (_, _) => RequestScrollToEnd();
    }

    protected override void OnSourceInitialized(EventArgs e)
    {
        base.OnSourceInitialized(e);
        EnableRoundedCorners();
    }

    private async void OnLoaded(object sender, RoutedEventArgs e)
    {
        await _vm.InitializeAsync();
    }

    // ---------- 日志自动滚动（节流）----------
    private void RequestScrollToEnd()
    {
        if (_autoScrollPending) return;
        _autoScrollPending = true;
        Dispatcher.BeginInvoke(new Action(() =>
        {
            _autoScrollPending = false;
            LogScroll?.ScrollToEnd();
        }), DispatcherPriority.Background);
    }

    // ---------- Win11 圆角 ----------
    [DllImport("dwmapi.dll")]
    private static extern int DwmSetWindowAttribute(IntPtr hwnd, int attr, ref int value, int size);

    private void EnableRoundedCorners()
    {
        try
        {
            var hwnd = new System.Windows.Interop.WindowInteropHelper(this).Handle;
            if (hwnd == IntPtr.Zero) return;
            int preference = 2; // DWMWCP_ROUND
            _ = DwmSetWindowAttribute(hwnd, 33 /* DWMWA_WINDOW_CORNER_PREFERENCE */,
                ref preference, sizeof(int));
        }
        catch
        {
            // Win10 或不支持时保持直角
        }
    }

    // ---------- 窗口控制 ----------
    private void OnMinimize(object sender, RoutedEventArgs e) => WindowState = WindowState.Minimized;

    private void OnMaximize(object sender, RoutedEventArgs e) =>
        WindowState = WindowState == WindowState.Maximized ? WindowState.Normal : WindowState.Maximized;

    private void OnClose(object sender, RoutedEventArgs e) => Close();

    private void UpdateMaximizeGlyph()
    {
        if (MaxButton == null) return;
        MaxButton.Content = WindowState == WindowState.Maximized ? "\uE923" : "\uE922";
    }

    protected override void OnClosing(CancelEventArgs e)
    {
        base.OnClosing(e);
        try { _vm.SaveConfig(); } catch { /* 关闭时保存失败不阻塞退出 */ }
    }

    // ---------- 验收截图（--screenshot）----------
    public void CaptureAfterLoaded(string path)
    {
        Dispatcher.BeginInvoke(new Action(async () =>
        {
            await Task.Delay(5000);
            try { Capture(path); }
            catch (Exception ex)
            {
                try { File.WriteAllText(path + ".error.txt", ex.ToString()); } catch { }
            }
            Application.Current.Shutdown();
        }), DispatcherPriority.ApplicationIdle);
    }

    private void Capture(string path)
    {
        if (Content is not FrameworkElement root) return;

        root.UpdateLayout();
        var dpi = VisualTreeHelper.GetDpi(this);
        int pw = Math.Max(1, (int)Math.Ceiling(root.ActualWidth * dpi.DpiScaleX));
        int ph = Math.Max(1, (int)Math.Ceiling(root.ActualHeight * dpi.DpiScaleY));

        var rtb = new RenderTargetBitmap(pw, ph, dpi.PixelsPerInchX, dpi.PixelsPerInchY, PixelFormats.Pbgra32);
        rtb.Render(root);

        var encoder = new PngBitmapEncoder();
        encoder.Frames.Add(BitmapFrame.Create(rtb));

        Directory.CreateDirectory(Path.GetDirectoryName(Path.GetFullPath(path))!);
        using var fs = File.Create(path);
        encoder.Save(fs);
    }
}
