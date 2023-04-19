<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta http-equiv="X-UA-Compatible" content="ie=edge">
    <title>夜莺告警通知</title>
    <style type="text/css">
        .wrapper {
            background-color: #f8f8f8;
            padding: 15px;
            height: 100%;
        }
        .main {
            width: 600px;
            padding: 30px;
            margin: 0 auto;
            background-color: #fff;
            font-size: 12px;
            font-family: verdana,'Microsoft YaHei',Consolas,'Deja Vu Sans Mono','Bitstream Vera Sans Mono';
        }
        header {
            border-radius: 2px 2px 0 0;
        }
        header .title {
            font-size: 14px;
            color: #333333;
            margin: 0;
        }
        header .sub-desc {
            color: #333;
            font-size: 14px;
            margin-top: 6px;
            margin-bottom: 0;
        }
        hr {
            margin: 20px 0;
            height: 0;
            border: none;
            border-top: 1px solid #e5e5e5;
        }
        em {
            font-weight: 600;
        }
        table {
            margin: 20px 0;
            width: 100%;
        }

        table tbody tr{
            font-weight: 200;
            font-size: 12px;
            color: #666;
            height: 32px;
        }

        .succ {
            background-color: green;
            color: #fff;
        }

        .fail {
            background-color: red;
            color: #fff;
        }

        .succ th, .succ td, .fail th, .fail td {
            color: #fff;
        }

        table tbody tr th {
            width: 80px;
            text-align: right;
        }
        .text-right {
            text-align: right;
        }
        .body {
            margin-top: 24px;
        }
        .body-text {
            color: #666666;
            -webkit-font-smoothing: antialiased;
        }
        .body-extra {
            -webkit-font-smoothing: antialiased;
        }
        .body-extra.text-right a {
            text-decoration: none;
            color: #333;
        }
        .body-extra.text-right a:hover {
            color: #666;
        }
        .button {
            width: 200px;
            height: 50px;
            margin-top: 20px;
            text-align: center;
            border-radius: 2px;
            background: #2D77EE;
            line-height: 50px;
            font-size: 20px;
            color: #FFFFFF;
            cursor: pointer;
        }
        .button:hover {
            background: rgb(25, 115, 255);
            border-color: rgb(25, 115, 255);
            color: #fff;
        }
        footer {
            margin-top: 10px;
            text-align: right;
        }
        .footer-logo {
            text-align: right;
        }
        .footer-logo-image {
            width: 108px;
            height: 27px;
            margin-right: 10px;
        }
        .copyright {
            margin-top: 10px;
            font-size: 12px;
            text-align: right;
            color: #999;
            -webkit-font-smoothing: antialiased;
        }
    </style>
</head>
<body>
<div class="wrapper">
    <div class="main">
        <header>
            <h3 class="title">{{.RuleName}}</h3>
            <p class="sub-desc"></p>
        </header>

        <hr>

        <div class="body">
            <table cellspacing="0" cellpadding="0" border="0">
                <tbody>
                {{if .IsRecovered}}
                <tr class="succ">
                    <th>级别状态：</th>
                    <td>S{{.Severity}} Recovered</td>
                </tr>
                {{else}}
                <tr class="fail">
                    <th>级别状态：</th>
                    <td>S{{.Severity}} Triggered</td>
                </tr>
                {{end}}

                <tr>
                    <th>策略备注：</th>
                    <td>{{.RuleNote}}</td>
                </tr>
                <tr>
                    <th>设备备注：</th>
                    <td>{{.TargetNote}}</td>
                </tr>
                {{if not .IsRecovered}}
                <tr>
                    <th>触发时值：</th>
                    <td>{{.TriggerValue}}</td>
                </tr>
                {{end}}

                {{if .TargetIdent}}
                <tr>
                    <th>监控对象：</th>
                    <td>{{.TargetIdent}}</td>
                </tr>
                {{end}}
                <tr>
                    <th>监控指标：</th>
                    <td>{{.TagsJSON}}</td>
                </tr>

                {{if .IsRecovered}}
                <tr>
                    <th>恢复时间：</th>
                    <td>{{timeformat .LastEvalTime}}</td>
                </tr>
                {{else}}
                <tr>
                    <th>触发时间：</th>
                    <td>
                        {{timeformat .TriggerTime}}
                    </td>
                </tr>
                {{end}}

                <tr>
                    <th>发送时间：</th>
                    <td>
                        {{timestamp}}
                    </td>
                </tr>

                <tr>
                    <th>PromQL：</th>
                    <td>
                        {{.PromQl}}
                    </td>
                </tr>
                </tbody>
            </table>

            <hr>

            <footer>
                <div class="copyright" style="font-style: italic">
                    我们希望与您一起，将监控这个事情，做到极致！
                </div>
            </footer>
        </div>
    </div>
</div>
</body>
</html>