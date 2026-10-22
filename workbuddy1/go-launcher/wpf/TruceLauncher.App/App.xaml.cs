using System.IO;
using System.Windows;
using System.Windows.Threading;

namespace TruceLauncher;

public partial class App : Application
{
    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);

        DispatcherUnhandledException += (_, args) =>
        {
            WriteCrash(args.Exception);
            args.Handled = true;
            MessageBox.Show("启动器遇到一个错误：\n\n" + args.Exception.Message,
                "Truce Launcher", MessageBoxButton.OK, MessageBoxImage.Warning);
        };
        AppDomain.CurrentDomain.UnhandledException += (_, args) =>
            WriteCrash(args.ExceptionObject as Exception);

        var win = new MainWindow();
        MainWindow = win;
        win.Show();

        // 开发/验收用：--screenshot <png 路径>，渲染完成后输出界面截图再退出。
        for (int i = 0; i < e.Args.Length - 1; i++)
        {
            if (e.Args[i] == "--screenshot")
            {
                win.CaptureAfterLoaded(e.Args[i + 1]);
                break;
            }
        }
    }

    private static void WriteCrash(Exception? ex)
    {
        if (ex == null) return;
        try
        {
            var dir = Path.Combine(
                Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                "TruceLauncher", "logs");
            Directory.CreateDirectory(dir);
            File.AppendAllText(Path.Combine(dir, "crash.log"),
                $"[{DateTime.Now:yyyy-MM-dd HH:mm:ss}] {ex}\n\n");
        }
        catch
        {
            // 日志失败不影响主流程
        }
    }
}
