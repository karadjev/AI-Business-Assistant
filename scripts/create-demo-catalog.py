"""Generate deterministic, synthetic-only XLSX fixture with no third-party dependencies."""
from pathlib import Path
from zipfile import ZipFile, ZipInfo, ZIP_DEFLATED
from io import BytesIO
from xml.sax.saxutils import escape
headers=['sku','name','category','brand','model','description','price','currency','quantity','availability','attributes']
rows=[headers,
 ['DEMO-IPHONE','iPhone — SYNTHETIC','Смартфоны','Apple','DEMO iPhone','SYNTHETIC: тестовые цена и остаток','59990.00','RUB','5','true','{"color":"test black"}'],
 ['DEMO-SAMSUNG','Samsung — SYNTHETIC','Смартфоны','Samsung','DEMO Galaxy','SYNTHETIC: тестовые цена и остаток','39990.00','RUB','7','true','{}'],
 ['DEMO-XIAOMI','Xiaomi — SYNTHETIC','Смартфоны','Xiaomi','DEMO Redmi','SYNTHETIC: тестовые цена и остаток','19990.50','RUB','9','true','{}'],
 ['DEMO-HEADPHONES','Наушники — SYNTHETIC','Наушники','Demo','DEMO Audio','SYNTHETIC: тестовые цена и остаток','2990.00','RUB','4','true','{}'],
 ['DEMO-CASE','Чехол — SYNTHETIC','Аксессуары','Demo','DEMO Case','SYNTHETIC: тестовые цена и остаток','990.00','RUB','0','false','{}'],
 ['DEMO-CHARGER','Зарядное устройство — SYNTHETIC','Зарядные устройства','Demo','DEMO Charger','SYNTHETIC: тестовые цена и остаток','1490.00','RUB','12','true','{}']]
worksheet='<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>'
for n,row in enumerate(rows,1):
 worksheet+=f'<row r="{n}">'+''.join(f'<c r="{chr(65+i)}{n}" t="inlineStr"><is><t>{escape(str(v))}</t></is></c>' for i,v in enumerate(row))+'</row>'
worksheet+='</sheetData></worksheet>'
parts={
 '[Content_Types].xml':'<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
 '_rels/.rels':'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>',
 'xl/workbook.xml':'<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="SYNTHETIC" sheetId="1" r:id="rId1"/></sheets></workbook>',
 'xl/_rels/workbook.xml.rels':'<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>',
 'xl/worksheets/sheet1.xml':worksheet}
buf=BytesIO()
with ZipFile(buf,'w',compression=ZIP_DEFLATED) as z:
 for name,data in sorted(parts.items()):
  info=ZipInfo(name,(2026,1,1,0,0,0));info.compress_type=ZIP_DEFLATED;z.writestr(info,data.encode())
for target in ['apps/web/public/fixtures/synthetic-electronics.xlsx','apps/api/internal/catalog/testdata/synthetic-electronics.xlsx']:
 p=Path(target);p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(buf.getvalue())
print('Generated synthetic-only XLSX:',len(buf.getvalue()),'bytes')
