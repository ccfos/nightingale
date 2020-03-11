import XLSX from 'xlsx';

export default function exportXlsx(data: any, filename = 'export') {
  const wb = XLSX.utils.book_new();
  const ws = XLSX.utils.json_to_sheet(data);

  XLSX.utils.book_append_sheet(wb, ws, filename);
  XLSX.writeFile(wb, `${filename}.xlsx`);
}
