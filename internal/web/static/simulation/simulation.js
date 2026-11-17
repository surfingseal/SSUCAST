import { fields, groups, questions, universities } from "./data.js";

const get = (id) => document.getElementById(id);
const labels = new Map([
  ["activity", ""],
  ["notice", "02 / 07 · 공고 읽기"],
  ["conditions", "03–04 / 07 · 조건 확인"],
  ["application", "05–06 / 07 · 대학 선택과 모의지원"],
  ["interview", "07 / 07 · 모의면접"],
  ["finished", "시뮬레이션 완료"],
]);

const conditionGroups = [
  {
    title: "지원 조건",
    items: [
      ["한 학기 이상 수료한 학부생", "휴학생이라면 파견학기인 2027-1학기에 복학해야 해요."],
      ["파견학기가 마지막 학기가 아님", "마지막 학기에는 파견될 수 없어요."],
      ["평점평균 2.5 / 4.5 이상", "증명용 성적증명서를 기준으로 확인해요."],
      ["해외여행에 결격사유가 없음", "파견 대상 대학의 추가 지원 조건도 확인해요."],
    ],
  },
  {
    title: "공인어학성적",
    items: [
      ["파견대학이 요구하는 어학 기준 확인", "시험 종류와 기준 점수는 대학마다 달라요. 붙임1을 확인해요."],
      ["어학성적이 제출일 기준 2년 이내", "유효기간 표기가 없는 성적도 2년 이내여야 해요."],
      ["면접일에 제출할 어학성적 원본 확인", "TOEIC을 제외한 공인외국어성적 원본을 지참해요."],
    ],
  },
];

const letter = "ABCD";
const filled = new Set();
const groupViews = new Map();
const schoolByCode = new Map(universities.map((school) => [school.code, school]));
const answers = Array(questions.length).fill(null);
const dialog = get("school-dialog");
const content = get("simulation-content");
const paper = get("paper-scroll");
const schoolPicker = paper.querySelector(".sim-school-field");
const fieldViews = new Map([...paper.querySelectorAll("[data-field]")].map((view) => {
  const group = view.closest("[data-form-group]")?.dataset.formGroup ?? "schools";
  return [Number(view.dataset.field), { view, group }];
}));
let stage = "activity";
let rankedSchools = [];
let questionIndex = 0;

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text) node.textContent = text;
  return node;
}

function button(className, text) {
  const node = element("button", className, text);
  node.type = "button";
  return node;
}

function showStage(next) {
  const previous = stage;
  get(stage).hidden = true;
  stage = next;
  const section = get(stage);
  section.hidden = false;
  if (next === "application" && previous === "conditions") {
    paper.scrollTo(0, 0);
  }
  const chip = get("stage-label");
  chip.textContent = labels.get(next);
  chip.hidden = next === "activity";
  section.querySelector(".sim-heading h2").focus({ preventScroll: true });
  content.scrollTop = 0;
}

function renderConditions() {
  for (const { title, items } of conditionGroups) {
    const panel = element("fieldset", "sim-condition-group");
    panel.append(element("legend", "", title));
    for (const [label, detail] of items) {
      const row = element("label", "sim-check-row");
      const control = element("input");
      control.type = "checkbox";
      const text = element("span");
      text.append(element("strong", "", label), element("small", "", detail));
      row.append(control, text);
      panel.append(row);
    }
    get("condition-groups").append(panel);
  }
}

function updateConditions() {
  const checks = [...get("condition-groups").querySelectorAll("input")];
  get("conditions-next").disabled = !checks.every((check) => check.checked);
}

function openSchoolPicker() {
  if (dialog.open) return;
  dialog.showModal();
  const list = get("school-list");
  list.scrollTop = 0;
  const selected = list.querySelector('[aria-pressed="true"]');
  if (selected) selected.scrollIntoView({ block: "nearest" });
}

function bindApplication() {
  schoolPicker.addEventListener("click", openSchoolPicker);
  for (const group of groups) {
    const area = paper.querySelector(`[data-form-group="${group.k}"]`);
    const target = area?.querySelector(".sim-form-area") ?? schoolPicker;
    if (area) {
      target.setAttribute("aria-label", `${group.name}에 예시 입력`);
      area.addEventListener("click", () => {
        filled.add(group.k);
        updateApplication();
      });
    }
    const item = element("li");
    const shortcut = button("sim-group-link", group.name);
    const status = element("span", "sim-group-status");
    shortcut.append(status);
    shortcut.addEventListener("click", () => {
      target.scrollIntoView({ block: "center", inline: "nearest" });
      target.focus({ preventScroll: true });
    });
    item.append(shortcut);
    get("application-groups").append(item);
    groupViews.set(group.k, { area: area ?? target, item, status });
  }
}

function schoolValues() {
  if (!rankedSchools.length) return new Map();
  const [first, second, third] = rankedSchools;
  return new Map([
    [-1, first.english], [-2, first.program], [-3, first.code],
    [4, second.program], [5, second.english], [6, second.code],
    [8, third.program], [9, third.english], [10, third.code],
  ]);
}

function updateApplication() {
  const values = schoolValues();
  for (const field of fields) {
    const { view, group } = fieldViews.get(field.k);
    const value = filled.has(group) ? values.get(field.k) ?? field.value ?? "" : "";
    view.textContent = value || view.dataset.placeholder || "";
    if (field.k === -1) {
      view.textContent = value || "대학 선택";
      schoolPicker.setAttribute("aria-label", value ? `1지망 ${value}, 대학 변경` : "1지망 대학 선택");
    } else if (view.dataset.placeholder) {
      view.setAttribute("aria-label", value ? `${field.name}: ${value}` : view.dataset.placeholder);
    } else if (value && view.tagName === "TD") {
      view.setAttribute("aria-label", `${field.name}: ${value}`);
    } else if (view.tagName === "TD") {
      view.removeAttribute("aria-label");
    }
  }
  for (const [key, { area, item, status }] of groupViews) {
    const complete = filled.has(key);
    area.dataset.filled = String(complete);
    item.dataset.filled = String(complete);
    status.textContent = complete ? "완료" : "미작성";
  }

  const first = rankedSchools[0];
  get("selected-school").textContent = first?.name ?? "대학 미선택";
  for (const option of get("school-list").querySelectorAll("button")) {
    option.setAttribute("aria-pressed", String(option.dataset.code === first?.code));
  }
  const allDone = filled.size === groups.length;
  get("application-progress").textContent = allDone
    ? "지원서 작성을 완료했어요."
    : `대학을 선택하고 모든 구역을 작성하세요. (${filled.size} / ${groups.length})`;
  get("application-next").disabled = !allDone;
}

function renderSchools() {
  const list = get("school-list");
  const regions = new Map();
  for (const school of universities) {
    if (!regions.has(school.region)) regions.set(school.region, []);
    regions.get(school.region).push(school);
  }
  for (const [region, schools] of regions) {
    const group = element("section", "sim-school-region");
    group.append(element("h3", "", `${region} · ${schools.length}개`));
    for (const school of schools) {
      const option = button("sim-school-option");
      option.dataset.code = school.code;
      const description = element("span");
      description.append(element("strong", "", school.name), element("small", "", school.location));
      option.append(description, element("span", "sim-school-code", school.code));
      option.addEventListener("click", () => {
        rankedSchools = [school, ...school.alternates.map((code) => schoolByCode.get(code))];
        filled.add("schools");
        updateApplication();
        dialog.close();
      });
      group.append(option);
    }
    list.append(group);
  }
}

function renderQuestion() {
  const question = questions[questionIndex];
  get("interview-question").textContent = question.title;
  get("interview-progress").textContent = `질문 ${questionIndex + 1} / ${questions.length}`;
  document.querySelectorAll(".sim-progress-bars span").forEach((bar, index) => {
    bar.classList.toggle("reached", index <= questionIndex);
  });

  const options = get("answer-options");
  options.replaceChildren();
  question.options.forEach((value, i) => {
    const row = element("label", "sim-answer");
    const control = element("input");
    control.type = "radio";
    control.name = "interview-answer";
    control.value = letter[i];
    control.checked = answers[questionIndex] === control.value;
    row.append(control, element("strong", "sim-option-letter", control.value), element("span", "", value));
    options.append(row);
  });
  updateFeedback();
}

function updateFeedback() {
  const question = questions[questionIndex];
  const selected = answers[questionIndex];
  const correct = selected === question.answer;
  const message = get("answer-message");
  message.hidden = selected === null;
  message.dataset.result = correct ? "correct" : "incorrect";
  message.textContent = selected === null ? "" : correct ? "정답이에요." : "오답입니다. 다시 선택해 주세요.";
  const correctLetter = get("correct-letter");
  correctLetter.hidden = !correct;
  correctLetter.textContent = correct ? `정답 ${question.answer}` : "";
  get("answer-guide").hidden = !correct;
  get("guide-text").textContent = correct ? question.guide : "";
  get("interview-feedback").classList.toggle("has-answer", selected !== null);

  for (const input of get("answer-options").querySelectorAll("input")) {
    input.closest("label").dataset.result = input.checked ? message.dataset.result : "";
  }
  get("interview-next").disabled = !correct;
}

renderConditions();
bindApplication();
renderSchools();
updateApplication();

const activity = get("select-exchange");
activity.checked = false;
activity.addEventListener("change", () => {
  get("activity-next").disabled = !activity.checked;
});
get("activity-next").addEventListener("click", () => showStage("notice"));
get("notice-next").addEventListener("click", () => showStage("conditions"));
get("condition-groups").addEventListener("change", updateConditions);
get("conditions-next").addEventListener("click", () => showStage("application"));
get("application-next").addEventListener("click", () => {
  renderQuestion();
  showStage("interview");
});
get("school-shortcut").addEventListener("click", openSchoolPicker);
document.querySelectorAll("[data-back]").forEach((control) => {
  control.addEventListener("click", () => showStage(control.dataset.back));
});
get("answer-options").addEventListener("change", (event) => {
  answers[questionIndex] = event.target.value;
  updateFeedback();
});
get("interview-next").addEventListener("click", () => {
  if (answers[questionIndex] !== questions[questionIndex].answer) return;
  if (questionIndex === questions.length - 1) {
    showStage("finished");
  } else {
    questionIndex += 1;
    renderQuestion();
    get("interview-question").focus();
    content.scrollTop = 0;
  }
});
get("interview-back").addEventListener("click", () => {
  if (questionIndex === 0) {
    showStage("application");
  } else {
    questionIndex -= 1;
    renderQuestion();
    get("interview-question").focus();
    content.scrollTop = 0;
  }
});
get("restart").addEventListener("click", () => {
  activity.checked = false;
  get("activity-next").disabled = true;
  for (const check of get("condition-groups").querySelectorAll("input")) check.checked = false;
  updateConditions();
  rankedSchools = [];
  filled.clear();
  updateApplication();
  answers.fill(null);
  questionIndex = 0;
  showStage("activity");
});
