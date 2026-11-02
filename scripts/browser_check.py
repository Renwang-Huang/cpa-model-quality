#!/usr/bin/env python3
"""Exercise the isolated synthetic CPA environment started with integration.py --keep."""
import json,pathlib,os
from playwright.sync_api import sync_playwright,expect
ROOT=pathlib.Path(__file__).resolve().parents[1]
STAGE=ROOT/'build/integration'
env=json.loads((STAGE/'environment.json').read_text())
checks=[]
def check(label):checks.append(label);print('PASS '+label,flush=True)
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path=os.environ.get("PLAYWRIGHT_CHROMIUM_EXECUTABLE",p.chromium.executable_path),headless=True)
 page=browser.new_page(viewport={'width':1440,'height':1050},color_scheme='light',accept_downloads=True)
 errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
 page.goto(env['base']+'/v0/resource/plugins/model-quality/page')
 page.locator('#key').fill(env['key']);page.locator('#login-form button').click();expect(page.locator('#accounts input')).to_have_count(3)
 expect(page.locator('#login')).to_be_hidden();expect(page.locator('#cases input')).to_have_count(12)
 assert page.locator('#inspect-case').count()==0 and page.locator('.footnote').count()==0
 check('setup reference panel and model-identity footnote are removed')
 # The host may use a dark theme while the OS prefers light, or vice versa.
 # Native popup colors must follow the host, including opaque option backgrounds.
 for os_theme,host_theme in [('light','dark'),('dark','light')]:
  page.emulate_media(color_scheme=os_theme)
  page.evaluate("""(theme) => {
    const root=document.documentElement;
    root.dataset.cpampPluginHost='true';root.dataset.theme=theme;
    const palette=theme==='dark'?{bg:'#171b28',surface:'#202636',field:'rgba(255,255,255,0.08)',ink:'#e5e5e5'}:{bg:'#f5f7fb',surface:'#fff',field:'rgba(0,0,0,0.04)',ink:'#20283b'};
    for(const [name,value] of Object.entries({'--bg-secondary':palette.bg,'--bg-primary':palette.surface,'--bg-tertiary':palette.field,'--border-color':'#778295','--text-primary':palette.ink,'--text-secondary':'#778295','--primary-color':'#4878f5','--success-color':'#158c6c','--danger-color':'#d7505e'}))root.style.setProperty(name,value);
  }""",host_theme)
  native=page.locator('#effort').evaluate("""(select) => ({scheme:getComputedStyle(select).colorScheme,color:getComputedStyle(select.options[0]).color,background:getComputedStyle(select.options[0]).backgroundColor})""")
  assert native['scheme']==host_theme,native
  assert native['background']==('rgb(28, 37, 53)' if host_theme=='dark' else 'rgb(255, 255, 255)'),native
  assert native['color']==('rgb(229, 229, 229)' if host_theme=='dark' else 'rgb(32, 40, 59)'),native
  page.locator('#effort').select_option('medium');expect(page.locator('#effort')).to_have_value('medium')
  page.screenshot(path=str(STAGE/f'host-{host_theme}-os-{os_theme}.png'),full_page=True)
 page.evaluate("() => {const root=document.documentElement;delete root.dataset.cpampPluginHost;delete root.dataset.theme;root.removeAttribute('style')}")
 page.emulate_media(color_scheme='light');page.locator('#effort').select_option('auto')
 check('native select colors follow opposite host and OS themes without losing selection')
 assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
 page.screenshot(path=str(STAGE/'desktop-setup.png'),full_page=True)
 check('authenticated bootstrap and desktop layout')
 page.locator('#select-accounts').click()
 for chip in page.locator('#models .selected').all():chip.click()
 page.get_by_role('button',name='quality-test',exact=True).click()
 for i in range(page.locator('#cases input').count()):page.locator('#cases input').nth(i).uncheck()
 for title in ['组合概率','代码追踪','鹈鹕骑自行车','SimpleBench · 两位说谎者']:
  page.locator('#cases label').filter(has_text=title).locator('input').check()
 expect(page.locator('#start')).to_have_text('开始 8 次测试');page.locator('#start').click()
 expect(page.locator('#batch-title')).to_contain_text('已完成',timeout=20000)
 expect(page.locator('.result-card')).to_have_count(8)
 assert '100.0' in page.locator('#batch-summary').inner_text()
 check('UI selection, request estimate, start, streaming completion and score summary')
 page.locator('#result-case').select_option('probability');expect(page.locator('.result-card:visible')).to_have_count(2)
 page.locator('#only-differences').check();expect(page.locator('.result-card:visible')).to_have_count(0);page.locator('#only-differences').uncheck()
 page.locator('.result-card:visible').first.get_by_text('完整回复',exact=True).click()
 expect(page.locator('#artifact-dialog')).to_be_visible();assert '"valid": 120' in page.locator('.raw-reply').inner_text()
 page.locator('#close-artifact').click();check('result filtering and raw-reply dialog')
 page.locator('#result-case').select_option('pelican')
 for button in page.locator('.result-card:visible').get_by_text('并排预览',exact=True).all():button.click()
 expect(page.locator('.result-card:visible iframe')).to_have_count(2)
 for iframe in page.locator('.result-card:visible iframe').all():
  frame=iframe.element_handle().content_frame();frame.wait_for_function('() => window.syntheticAnimation === true')
  assert frame.evaluate('() => { try { void parent.document; return false; } catch(e) { return e.name === "SecurityError"; } }')
  assert frame.evaluate('async () => { try { await fetch("http://127.0.0.1:28418/blocked-network"); return false; } catch(e) { return true; } }')
 page.screenshot(path=str(STAGE/'desktop-results.png'),full_page=True)
 check('side-by-side scripted previews execute with opaque origin and blocked network')
 page.locator('#baseline').click();expect(page.locator('#baseline')).to_have_text('当前比较基线')
 with page.expect_download() as download:page.locator('#export').click()
 report=json.loads(download.value.path().read_text());assert len(report['batch']['items'])==8 and env['key'] not in json.dumps(report)
 check('baseline and downloaded JSON report')
 page.locator('#result-case').select_option('probability')
 page.locator('.result-card:visible').first.get_by_text('单项重测',exact=True).click()
 expect(page.locator('#batch-title')).to_contain_text('已完成',timeout=20000);expect(page.locator('.result-card')).to_have_count(1)
 expect(page.locator('.result-card')).to_contain_text('较基线 +0.0 分')
 check('single-result rerun preserves prior batch and compares the matching baseline')
 page.locator('[data-tab="history"]').click();expect(page.locator('.history-row')).not_to_have_count(0)
 assert '当前基线' in page.locator('#history-list').inner_text()
 page.screenshot(path=str(STAGE/'desktop-history.png'),full_page=True)
 page.emulate_media(color_scheme='dark');page.locator('[data-tab="setup"]').click();page.screenshot(path=str(STAGE/'desktop-dark.png'),full_page=True)
 page.set_viewport_size({'width':390,'height':844});assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
 page.screenshot(path=str(STAGE/'mobile-setup.png'),full_page=True)
 page.locator('[data-tab="results"]').click();assert not page.evaluate('document.documentElement.scrollWidth>innerWidth')
 page.screenshot(path=str(STAGE/'mobile-results.png'),full_page=True)
 check('history, dark theme, 390px setup and result layouts')
 page.set_viewport_size({'width':1440,'height':1050});page.locator('[data-tab="setup"]').click()
 for box in page.locator('#cases input').all():box.uncheck()
 page.locator('#custom-box').evaluate('(n) => n.open=true');page.locator('#custom-prompt').fill('Synthetic browser custom prompt');page.locator('#use-custom').check()
 expect(page.locator('#start')).to_have_text('开始 2 次测试');page.locator('#start').click()
 expect(page.locator('#batch-title')).to_contain_text('已完成',timeout=20000);expect(page.locator('.result-card')).to_have_count(2)
 assert '0/0' in page.locator('#batch-summary').inner_text()
 check('custom prompt runs without a fabricated score')
 page.locator('[data-tab="setup"]').click();page.locator('#accounts input').nth(1).uncheck()
 page.locator('#custom-prompt').fill('MOCK_WAIT');page.locator('#repeats').fill('3');page.locator('#concurrency').fill('1');page.locator('#start').click()
 expect(page.locator('#result-cards')).to_contain_text('stream started',timeout=15000);page.locator('#cancel').click()
 expect(page.locator('#batch-title')).to_contain_text('已取消',timeout=15000)
 expect(page.locator('.result-card')).to_have_count(3);assert '0/0' in page.locator('#batch-summary').inner_text()
 check('UI cancellation stops the stream and queued repetitions without scores')
 assert not errors,errors
 check('no browser page errors')
 browser.close()
(STAGE/'browser-verification.json').write_text(json.dumps({'checks':checks,'page_errors':errors,'real_model_requests':0},indent=2)+'\n')
