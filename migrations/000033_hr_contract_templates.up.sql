-- 000033_hr_contract_templates.up.sql
--
-- Reusable contract/agreement templates for the HR workspace. A template is the
-- authoring artefact: a French body with {{namespace.key}} placeholders. Issuing
-- a document (a later slice) will resolve those placeholders and freeze the
-- rendered result, exactly like an invoice freezes seller/buyer/lines — editing a
-- template must never alter an already-issued document.
--
-- Two kinds ship today:
--   employment — a full employment contract, always bound to an hr_employees row.
--   memo_deal  — a shorter memorandum of agreement, usable either with an employee
--                or with a free-text external party (contractor, driver, artist),
--                which is why its body uses only {{party.*}} and {{custom.*}} and
--                never {{employee.*}} / {{contract.*}}.
--
-- `field_labels` is an optional label map for the {{custom.*}} blanks. The body
-- stays the single source of truth for *which* blanks exist (they are discovered
-- by scanning it), so a label is only ever decoration and can never disagree.
-- Named field_labels rather than `fields` to keep the unquoted column lists the
-- HR resource engine builds well clear of MySQL keywords, per the `rank` lesson.
--
-- French text uses the typographic apostrophe (U+2019) throughout. That matches
-- the console's French copy and, usefully, needs no SQL escaping.

CREATE TABLE hr_contract_templates (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    kind         ENUM('employment','memo_deal') NOT NULL,
    code         VARCHAR(50)  NOT NULL,
    name         VARCHAR(150) NOT NULL,
    description  TEXT NULL,
    body         MEDIUMTEXT NOT NULL,
    field_labels JSON NULL,
    version      INT NOT NULL DEFAULT 1,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_hr_contract_templates_code (code),
    KEY idx_hr_contract_templates_kind (kind, is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Starter bodies: a deliberately plain skeleton of the clauses common to any
-- employer/employee agreement. They are a starting point for the organisation's
-- own legal review, not vetted legal text.
INSERT INTO hr_contract_templates (kind, code, name, description, body, field_labels) VALUES
('employment', 'EMPLOYMENT_STD', 'Contrat de travail (standard)',
 'Squelette de contrat de travail reprenant les clauses communes employeur / salarié.',
 'CONTRAT DE TRAVAIL

ENTRE LES SOUSSIGNÉS

{{employer.legal_name}}, dont le siège social est situé {{employer.address}}, {{employer.city}},
immatriculée sous le NIF {{employer.nif}} et le numéro statistique {{employer.stat}},
ci-après dénommée « l’Employeur »,

D’UNE PART,

ET

{{party.full_name}}, demeurant {{party.address}},
titulaire de la pièce d’identité n° {{party.id_number}},
ci-après dénommé(e) « le Salarié »,

D’AUTRE PART,

IL A ÉTÉ CONVENU CE QUI SUIT :

ARTICLE 1 — ENGAGEMENT
L’Employeur engage le Salarié, qui accepte, au poste de {{employee.position_title}}
au sein du département {{employee.department_name}}, sous la responsabilité de
{{employee.manager_name}}. Le matricule attribué au Salarié est {{employee.employee_number}}.

ARTICLE 2 — NATURE ET DURÉE DU CONTRAT
Le présent contrat est conclu pour une durée de type {{contract.type}}.
Il prend effet le {{contract.start_date}} et, le cas échéant, prend fin le {{contract.end_date}}.

ARTICLE 3 — PÉRIODE D’ESSAI
Le Salarié est soumis à une période d’essai prenant fin le {{contract.probation_end_date}}.
Durant cette période, chaque partie peut mettre fin au contrat dans les conditions prévues
par la législation en vigueur.

ARTICLE 4 — LIEU DE TRAVAIL
Les fonctions sont exercées à {{custom.work_place}}.

ARTICLE 5 — DURÉE DU TRAVAIL
La durée hebdomadaire de travail est fixée à {{custom.weekly_hours}} heures.

ARTICLE 6 — RÉMUNÉRATION
En contrepartie de ses fonctions, le Salarié perçoit une rémunération de
{{contract.salary}} {{contract.currency}}, versée selon une périodicité {{contract.pay_frequency}}.

ARTICLE 7 — CONGÉS
Le Salarié bénéficie des congés payés dans les conditions prévues par la législation
en vigueur et par les politiques internes de l’Employeur.

ARTICLE 8 — OBLIGATIONS ET CONFIDENTIALITÉ
Le Salarié s’engage à exercer ses fonctions avec diligence et loyauté, à respecter le
règlement intérieur, et à observer une stricte confidentialité sur l’ensemble des
informations dont il a connaissance dans le cadre de ses fonctions.

ARTICLE 9 — RUPTURE DU CONTRAT
Le présent contrat peut être rompu par l’une ou l’autre des parties, sous réserve du
respect d’un préavis de {{custom.notice_period}} et des dispositions légales applicables.

ARTICLE 10 — DROIT APPLICABLE
Le présent contrat est régi par la législation du travail en vigueur.

Fait à {{doc.place}}, le {{doc.issue_date}}, en deux exemplaires originaux.


L’Employeur                                        Le Salarié
{{custom.signatory_name}}                          {{party.full_name}}
{{custom.signatory_title}}',
 '{"work_place":{"label":"Lieu de travail"},"weekly_hours":{"label":"Heures par semaine"},"notice_period":{"label":"Durée du préavis"},"signatory_name":{"label":"Signataire (employeur)"},"signatory_title":{"label":"Fonction du signataire"}}'),

('memo_deal', 'MEMO_DEAL_STD', 'Mémorandum d’accord (standard)',
 'Accord court reprenant les bases d’un engagement : objet, durée, rémunération, obligations et résiliation. Utilisable avec un collaborateur interne ou une partie externe.',
 'MÉMORANDUM D’ACCORD

Référence : {{doc.reference}}

ENTRE

{{employer.legal_name}}, dont le siège social est situé {{employer.address}}, {{employer.city}},
ci-après dénommée « l’Employeur »,

ET

{{party.full_name}}, demeurant {{party.address}},
titulaire de la pièce d’identité n° {{party.id_number}},
ci-après dénommé(e) « le Collaborateur »,

IL A ÉTÉ CONVENU CE QUI SUIT :

ARTICLE 1 — OBJET
Le présent accord définit les conditions dans lesquelles le Collaborateur réalise la
mission suivante : {{custom.mission}}.

ARTICLE 2 — DURÉE
L’accord prend effet le {{custom.start_date}} et s’achève le {{custom.end_date}}.

ARTICLE 3 — LIEU D’EXÉCUTION
La mission est exécutée à {{custom.work_place}}.

ARTICLE 4 — RÉMUNÉRATION
En contrepartie, l’Employeur verse au Collaborateur la somme de
{{custom.amount}} {{employer.currency}}, selon les modalités suivantes :
{{custom.payment_terms}}.

ARTICLE 5 — OBLIGATIONS DU COLLABORATEUR
Le Collaborateur s’engage à exécuter la mission avec diligence, à respecter les consignes
d’organisation et de sécurité de l’Employeur, et à rendre compte de son avancement.

ARTICLE 6 — CONFIDENTIALITÉ
Le Collaborateur observe une stricte confidentialité sur toute information obtenue à
l’occasion de la mission, pendant sa durée et après son terme.

ARTICLE 7 — RÉSILIATION
Chaque partie peut mettre fin au présent accord moyennant un préavis de
{{custom.notice_period}}, notifié par écrit.

ARTICLE 8 — DROIT APPLICABLE
Le présent accord est régi par la législation en vigueur.

Fait à {{doc.place}}, le {{doc.issue_date}}, en deux exemplaires originaux.


L’Employeur                                        Le Collaborateur
{{custom.signatory_name}}                          {{party.full_name}}
{{custom.signatory_title}}',
 '{"mission":{"label":"Objet de la mission"},"start_date":{"label":"Date de début"},"end_date":{"label":"Date de fin"},"work_place":{"label":"Lieu d’exécution"},"amount":{"label":"Montant"},"payment_terms":{"label":"Modalités de paiement"},"notice_period":{"label":"Durée du préavis"},"signatory_name":{"label":"Signataire (employeur)"},"signatory_title":{"label":"Fonction du signataire"}}');
